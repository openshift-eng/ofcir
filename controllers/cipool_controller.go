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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

// Additional permissions required by the controller
//+kubebuilder:rbac:groups="",namespace=ofcir-system,resources=secrets,verbs=get;list;watch;create;update;patch;delete

const (
	defaultCIPoolRetryDelay = time.Minute * 1
)

// CIPoolReconciler reconciles a CIPool object
type CIPoolReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=cipools,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=cipools/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=cipools/finalizers,verbs=update

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

	// Check if pool contains a finalizer when is not under deletion
	if pool.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if not present
		if !controllerutil.ContainsFinalizer(pool, ofcirv1.OfcirFinalizer) {
			logger.Info("Adding finalizer")
			controllerutil.AddFinalizer(pool, ofcirv1.OfcirFinalizer)
			if err := r.Update(ctx, pool, &client.UpdateOptions{}); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		// Delete has been requested
		cirs := &ofcirv1.CIResourceList{}
		r.List(context.TODO(), cirs, client.InNamespace(pool.Namespace))

		poolCirs := []ofcirv1.CIResource{}
		for _, c := range cirs.Items {
			if c.Spec.PoolRef.Name == pool.Name {
				poolCirs = append(poolCirs, c)
			}
		}

		// Still some resources to be deleted
		if len(poolCirs) > 0 {
			_, err = r.deleteCIResources(0, poolCirs, pool.Namespace, logger)
			if err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 10 * time.Second}, err
		}

		// No more resources, pool can be deleted
		if controllerutil.ContainsFinalizer(pool, ofcirv1.OfcirFinalizer) {
			controllerutil.RemoveFinalizer(pool, ofcirv1.OfcirFinalizer)
			logger.Info("Deleting pool")
			if err := r.Update(ctx, pool, &client.UpdateOptions{}); err != nil {
				return ctrl.Result{}, err
			}
		}

		return ctrl.Result{}, nil
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

	// Retrieve the pool secret, if defined
	poolSecretKey := types.NamespacedName{
		Namespace: pool.Namespace,
		Name:      fmt.Sprintf("%s-secret", pool.Name),
	}
	poolSecret := v1.Secret{}
	if err := r.Get(context.Background(), poolSecretKey, &poolSecret); err != nil {
		if !errors.IsNotFound(err) {
			return ctrl.Result{}, err
		}
	}

	// Pool is available
	isDirty, err := r.manageCIResourcesFor(pool, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	retryDelay := defaultCIPoolRetryDelay
	if isDirty {
		// In case of changes, force a quick re-evaluation of the pool
		retryDelay = 1 * time.Second
	}

	return ctrl.Result{RequeueAfter: retryDelay}, nil
}

func (r *CIPoolReconciler) manageCIResourcesFor(pool *ofcirv1.CIPool, logger logr.Logger) (bool, error) {
	// Retrieve all cirs
	allCirs := &ofcirv1.CIResourceList{}
	err := r.List(context.TODO(), allCirs, client.InNamespace(pool.Namespace))
	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to list CIResources in namespace: %s", pool.Namespace))
		return false, err
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

	// Update status if required with the current effective number of resources
	if pool.Status.Size != len(poolCirs) {
		pool.Status.Size = len(poolCirs)
		if err = r.savePoolStatus(pool); err != nil {
			logger.Error(err, "error while updating status")
			return false, err
		}
	}

	if pool.Spec.Size == len(poolCirs) {
		return false, nil
	}

	numCirSelected := 0

	if pool.Spec.Size > len(poolCirs) {
		logger.Info("Adding resources to the pool", "Expected", pool.Spec.Size, "Found", len(poolCirs))

		baseCirNo := r.getHighestResourceNumeral(allCirs.Items, logger) + 1

		for i := baseCirNo; i < baseCirNo+(pool.Spec.Size-len(poolCirs)); i++ {
			logger.Info("Creating new CIResource", "CIResource", i)
			if err = r.createCIResource(pool, i, logger); err != nil {
				logger.Error(err, "error while creating new CIResource", "CIResource", i)
				continue
			}
			numCirSelected++
		}
	} else {
		numCirSelected, err = r.deleteCIResources(pool.Spec.Size, poolCirs, pool.Namespace, logger)
		if err != nil {
			return false, err
		}
	}

	return numCirSelected > 0, err
}

func (r *CIPoolReconciler) deleteCIResources(targetSize int, poolCirs []ofcirv1.CIResource, poolNamespace string, logger logr.Logger) (int, error) {
	logger.Info("Removing resources from the pool", "Expected", targetSize, "Found", len(poolCirs))

	numCirSelected := 0

	// Select candidates for eviction starting from the newest resources
	for i := len(poolCirs) - 1; i >= 0; i-- {
		cir := poolCirs[i]

		switch cir.Status.State {
		case ofcirv1.StateAvailable, ofcirv1.StateMaintenance, ofcirv1.StateError:
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
				logger.Error(err, "error while selecting CIResource to be removed, skipping it", "CIResource", cir.Name)
				continue
			}

			// Check if enough instances have been selected for eviction
			numCirSelected++

		default:
			logger.Info("CIResource ignored for eviction", "CIResource", cir.Name, "State", cir.Status.State)
		}

		if numCirSelected >= len(poolCirs)-targetSize {
			break
		}
	}

	err := r.DeleteAllOf(context.TODO(), &ofcirv1.CIResource{}, client.InNamespace(poolNamespace), client.MatchingLabels{ofcirv1.EvictionLabel: "true"})
	if err != nil {
		logger.Error(err, "error while batch deleting CIResources")
		return numCirSelected, err
	}

	return numCirSelected, nil
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
			Type:  pool.Spec.Type,
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
