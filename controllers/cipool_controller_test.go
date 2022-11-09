package controllers

import (
	"context"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/reconcilertest"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func TestCIPoolController(t *testing.T) {

	cases := []struct {
		name     string
		testCase reconcilertest.Testable
	}{
		{
			name: "finalizer is immediately added for a new pool",
			testCase: newCIPoolScenario().
				Setup(scenarioPoolWithSingleCir).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIPool) bool {
					return obj.Status.State == ofcirv1.StatePoolAvailable && controllerutil.ContainsFinalizer(obj, ofcirv1.OfcirFinalizer)
				}),
		},
		{
			name: "pool resizing",
			testCase: newCIPoolScenario().
				Setup(scenarioWithEmptyPool).
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIPool) bool {
					return obj.Status.State == ofcirv1.StatePoolAvailable
				}, "start with an empty pool").
				Then(func(t *testing.T, client client.Client, obj *ofcirv1.CIPool) {
					assert.Equal(t, 0, obj.Status.Size)
					obj.Spec.Size = 5
					client.Update(context.Background(), obj)
				}, "resize the pool").
				ReconcileUntil(func(client client.Client, obj *ofcirv1.CIPool) bool {
					var cirs ofcirv1.CIResourceList
					client.List(context.Background(), &cirs)

					// Ensure the current pool size is correct and
					// enough cirs have been created
					return obj.Status.Size == 5 && len(cirs.Items) == 5
				}, "wait for 5 CIResources"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, tc.testCase.Test)
	}
}

func newCIPoolScenario() reconcilertest.Scenario[CIPoolReconciler, ofcirv1.CIPool, *ofcirv1.CIPool] {
	return reconcilertest.New[CIPoolReconciler, ofcirv1.CIPool]().
		WithSchemes(ofcirv1.AddToScheme, corev1.AddToScheme)
}
