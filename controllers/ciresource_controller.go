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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/go-logr/logr"
	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

// CIResourceReconciler reconciles a CIResource object
type CIResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=ciresources,verbs=get;list;watch;create;update;patch;delete;deletecollection
//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=ciresources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=ofcir.openshift,namespace=ofcir-system,resources=ciresources/finalizers,verbs=update

// Reconcile handles changes to the CIResource type
func (r *CIResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithName(req.NamespacedName.Name)

	cir := &ofcirv1.CIResource{}
	err := r.Get(ctx, req.NamespacedName, cir)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "could not get CIResource", "Name", req.Name)
		return ctrl.Result{}, nil
	}

	logger.Info("started", "State", cir.Status.State)

	pool, poolSecret, err := r.getPool(cir, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	fsm := NewCIResourceFSM(logger)
	isDirty, isStatusDirty, retryAfter, err := fsm.Process(cir, pool, poolSecret)
	if err == nil {
		if isDirty {
			err = r.updateResource(cir)
		} else if isStatusDirty {
			err = r.updateStatus(cir)
		}
	}
	if err != nil {
		logger.Error(err, "error while processing CIResource")
		return ctrl.Result{}, err
	}

	logger.Info("done", "State", cir.Status.State)
	return ctrl.Result{RequeueAfter: retryAfter}, nil
}

func (r *CIResourceReconciler) getPool(cir *ofcirv1.CIResource, logger logr.Logger) (*ofcirv1.CIPool, *v1.Secret, error) {

	pool := &ofcirv1.CIPool{}
	poolName := types.NamespacedName{Namespace: cir.Namespace, Name: cir.Spec.PoolRef.Name}
	err := r.Get(context.Background(), poolName, pool)
	if err != nil {
		logger.Error(err, "could not get CIPool", "Pool", cir.Spec.PoolRef)
		return nil, nil, err
	}

	// Retrieve the pool secret, if defined
	poolSecretKey := types.NamespacedName{
		Namespace: pool.Namespace,
		Name:      fmt.Sprintf("%s-secret", pool.Name),
	}
	poolSecret := &v1.Secret{}
	if err := r.Get(context.Background(), poolSecretKey, poolSecret); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "could not get CIPool secret", "Secret", cir.Spec.PoolRef)
			return nil, nil, err
		}
	}

	return pool, poolSecret, nil
}

func (r *CIResourceReconciler) updateResource(cir *ofcirv1.CIResource) error {
	return r.Update(context.TODO(), cir)
}

func (r *CIResourceReconciler) updateStatus(cir *ofcirv1.CIResource) error {
	t := metav1.Now()
	cir.Status.LastUpdated = &t

	return r.Status().Update(context.TODO(), cir)
}

// SetupWithManager sets up the controller with the Manager
func (r *CIResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ofcirv1.CIResource{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}
