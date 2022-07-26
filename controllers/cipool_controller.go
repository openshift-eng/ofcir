/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

const (
	defaultCIPoolRetryDelay = time.Minute * 1
)

// CIPoolReconciler reconciles a CIPool object
type CIPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ofcir.openshift,resources=cipools,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ofcir.openshift,resources=cipools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ofcir.openshift,resources=cipools/finalizers,verbs=update

// Reconcile handles changes to the CIPool type
func (r *CIPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.NamespacedName.Name)
	logger.Info("started")

	// Fetch the pool resource
	pool := &ofcirv1.CIPool{}
	err := r.Get(ctx, req.NamespacedName, pool)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Info("could not get CIPool")
		return ctrl.Result{RequeueAfter: defaultCIPoolRetryDelay}, nil
	}

	// Check if a state update is required
	if pool.Status.State != pool.Spec.State {
		switch pool.Spec.State {
		case ofcirv1.StatePoolAvailable, ofcirv1.StatePoolOffline:
			pool.Status.State = pool.Spec.State
			if err = r.savePoolStatus(pool); err != nil {
				logger.Error(err, "error while updating status")
			}
		default:
			err = fmt.Errorf("invalid state: %s", pool.Spec.State)
		}

		return ctrl.Result{}, err
	}

	// Check if the pool is offline, in such case let's skip the reconciliation
	if pool.Status.State == ofcirv1.StatePoolOffline {
		logger.Info("pool is offline, skipping")
		return ctrl.Result{RequeueAfter: defaultCIPoolRetryDelay}, nil
	}

	// Pool is available
	err = r.manageCIResourcesFor(pool, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: defaultCIPoolRetryDelay}, nil
}

func (r *CIPoolReconciler) manageCIResourcesFor(pool *ofcirv1.CIPool, logger logr.Logger) error {
	// Retrieve all cirs
	allCirs := &ofcirv1.CIResourceList{}
	err := r.List(context.TODO(), allCirs, client.InNamespace(pool.Namespace))
	if err != nil {
		logger.Error(err, "failed to list CIResources in namespace: %s", pool.Namespace)
		return err
	}

	sort.SliceStable(allCirs.Items, func(i, j int) bool {
		return allCirs.Items[i].Name < allCirs.Items[j].Name
	})

	// Filters out cirs not belonging to the current pool
	var poolCirs []ofcirv1.CIResource
	for _, c := range allCirs.Items {
		if c.Spec.PoolRef.Name == pool.Name {
			poolCirs = append(poolCirs, c)
		}
	}

	if pool.Spec.Size == len(poolCirs) {
		return nil
	}

	if pool.Spec.Size > len(poolCirs) {
		logger.Info("Adding resources to the pool", "Expected", pool.Spec.Size, "Found", len(poolCirs))

		baseCirNo := r.getHighestResourceNumeral(allCirs.Items, logger) + 1

		for i := baseCirNo; i < baseCirNo+(pool.Spec.Size-len(poolCirs)); i++ {
			logger.Info("Creating new CIResource", "CIResource", i)
			if err = r.createCIResource(pool, i, logger); err != nil {
				return err
			}
		}
	} else {
		logger.Info("Removing resources from the pool", "Expected", pool.Spec.Size, "Found", len(poolCirs))

		// Select candidates for eviction starting from the newest resources
		numCirSelected := 0
		for i := len(poolCirs) - 1; i >= 0; i-- {
			cir := poolCirs[i]

			switch cir.Status.State {
			case ofcirv1.StateAvailable, ofcirv1.StateMaintenance, ofcirv1.StateError, ofcirv1.StateNone:
				labels := cir.GetLabels()
				if labels == nil {
					labels = make(map[string]string)
				}
				if _, found := labels[ofcirv1.EvictionLabel]; found {
					continue
				}
				logger.Info("CIResource selected for eviction", "Name", cir.Name)

				labels[ofcirv1.EvictionLabel] = "true"
				cir.SetLabels(labels)

				if err := r.Update(context.TODO(), &cir); err != nil {
					logger.Error(err, "error while selecting CIResource to be removed", "CIResource", cir.Name)
					return err
				}

				// Chheck if enough instances have been selected for eviction
				numCirSelected++
			}

			if numCirSelected >= len(poolCirs)-pool.Spec.Size {
				break
			}
		}

		err = r.DeleteAllOf(context.TODO(), &ofcirv1.CIResource{}, client.InNamespace(pool.Namespace), client.MatchingLabels{ofcirv1.EvictionLabel: "true"})
		if err != nil {
			logger.Error(err, "error while batch deleting CIResources")
			return err
		}
	}

	return nil
}

func (r *CIPoolReconciler) createCIResource(pool *ofcirv1.CIPool, cirNo int, logger logr.Logger) error {

	cir := &ofcirv1.CIResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("cir-%04d", cirNo),
			Namespace: pool.Namespace,
		},
		Spec: ofcirv1.CIResourceSpec{
			PoolRef: v1.LocalObjectReference{
				Name: pool.Name,
			},
			State: ofcirv1.StateNone,
			Extra: "",
			Type:  ofcirv1.TypeCIHost,
		},
	}

	return r.Create(context.TODO(), cir)
}

func (r *CIPoolReconciler) getHighestResourceNumeral(cirs []ofcirv1.CIResource, logger logr.Logger) int {

	highestNumeral := 0
	for _, cir := range cirs {
		lastIndex := strings.LastIndex(cir.Name, "-")
		if lastIndex == -1 {
			logger.Info("CIResource name malformed, skipping", "CIResource", cir.Name)
			continue
		}
		currentNumeral, err := strconv.Atoi(cir.Name[lastIndex+1:])
		if err != nil {
			logger.Info("CIResource name malformed, skipping", "CIResource", cir.Name)
			continue
		}
		if highestNumeral < currentNumeral {
			highestNumeral = currentNumeral
		}
	}

	return highestNumeral
}

func (r *CIPoolReconciler) savePoolStatus(pool *ofcirv1.CIPool) error {
	t := metav1.Now()
	pool.Status.LastUpdated = &t

	return r.Status().Update(context.TODO(), pool)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CIPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			// The controller will perform batch create/delete on CIResources
			MaxConcurrentReconciles: 1,
		}).
		For(&ofcirv1.CIPool{}).
		Complete(r)
}
