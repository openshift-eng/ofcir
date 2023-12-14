package controllers

import (
	"context"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/reconcilertest"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestCIResourceCreationAndAcquisition(t *testing.T) {

	cases := []struct {
		name     string
		testCase reconcilertest.Testable
	}{
		{
			name: "finalizer is immediately added for a new resource",
			testCase: newCIResourceScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, cir *ofcirv1.CIResource) bool {
					return cir.Status.State == ofcirv1.StateNone && controllerutil.ContainsFinalizer(cir, ofcirv1.OfcirFinalizer)
				}),
		},
		{
			name: "new resource gets provisioned and becomes available with a valid id and address",
			testCase: newCIResourceScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateProvisioning
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateProvisioningWait
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					assert.NotEmpty(t, obj.Status.ResourceId)
					assert.NotEmpty(t, obj.Status.Address)
				}),
		},
		{
			name: "available resource can be moved under maintenance",
			testCase: newCIResourceScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateMaintenance
					client.Update(context.Background(), obj)
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateMaintenance
				}),
		},
		{
			name: "acquire and release an available resource",
			testCase: newCIResourceScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateInUse
					client.Update(context.Background(), obj)
				}, "acquire the available resource").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateInUse
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateAvailable
					client.Update(context.Background(), obj)
				}, "release the resource").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}),
		},
		{
			name: "released resource gets cleaned and becomes available",
			testCase: newCIResourceScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateInUse
					client.Update(context.Background(), obj)
				}, "acquire the available resource").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateInUse
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateAvailable
					client.Update(context.Background(), obj)
				}, "release the acquired resource").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateCleaning
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateCleaningWait
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.testCase.Test)
	}
}

func TestCIResourceReconcilerFallbacks(t *testing.T) {

	cases := []struct {
		name     string
		testCase reconcilertest.Testable
	}{
		{
			name: "delete available fallback resource",
			testCase: newCIResourceScenario().
				Setup(func() []runtime.Object {
					cip, secret := cipoolWithSecret()
					cip.priority(-1)
					cir := cir("cir-0").pool(cip.Name)

					return []runtime.Object{
						cir.build(), cip.build(), secret,
					}
				}).
				ReconcileUntil(func(client client.Client, cir *ofcirv1.CIResource) bool {
					return cir.Status.State == ofcirv1.StateAvailable
				}, "wait for cir to become available").
				Then(func(t *testing.T, client client.Client, cir *ofcirv1.CIResource) {
					client.Delete(context.TODO(), cir)
				}, "delete cir").
				ReconcileUntil(func(client client.Client, cir *ofcirv1.CIResource) bool {
					return cir == nil
				}, "wait for cir to be removed").Case(),
		},
		{
			name: "do not release fallback dummy resources (just cleanup)",
			testCase: newCIResourceScenario().
				Setup(func() []runtime.Object {
					cip, secret := cipoolWithSecret()
					cip.priority(-1)
					cir := cir("cir-0").pool(cip.Name)

					return []runtime.Object{
						cir.build(), cip.build(), secret,
					}
				}).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					obj.Spec.State = ofcirv1.StateInUse
					client.Update(context.Background(), obj)
				}, "acquire the fallback resource").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateInUse
				}).
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIResource) {
					// A premature release can happen if the provider didn't provisioned yet the instance,
					// thus the CIR resource is still marked with the dummy fallback id
					assert.Equal(t, fallbackResourceID, obj.Status.ResourceId)
					obj.Spec.State = ofcirv1.StateAvailable
					client.Update(context.Background(), obj)
				}, "release the resource before getting provisioned by the provider").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIResource) bool {
					return obj.Status.State == ofcirv1.StateAvailable
				}).Case(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.testCase.Test)
	}
}

func newCIResourceScenario() reconcilertest.Scenario[CIResourceReconciler, ofcirv1.CIResource, *ofcirv1.CIResource] {
	return reconcilertest.New[CIResourceReconciler, ofcirv1.CIResource]().
		WithSchemes(ofcirv1.AddToScheme, corev1.AddToScheme)
}

func scenarioPoolWithSingleCir() []runtime.Object {
	cip, secret := cipoolWithSecret()
	cir := cir("cir-0").pool(cip.Name)

	return []runtime.Object{
		cir.build(), cip.build(), secret,
	}
}

func scenarioWithEmptyPool() []runtime.Object {

	cip, secret := cipoolWithSecret()
	cip.size(0)

	return []runtime.Object{
		cip.build(), secret,
	}

}
