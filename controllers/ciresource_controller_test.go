package controllers

import (
	"context"
	"fmt"
	"testing"
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestCIResourceReconciler_Reconcile(t *testing.T) {

	tests := []struct {
		name   string
		cipool *cipoolBuilder
		cir    *cirBuilder

		waitUntil ofcirv1.CIResourceState

		expectedStates []ofcirv1.CIResourceState
		expectedError  error
	}{
		{
			name:      "new",
			cipool:    cipool(),
			cir:       cir("0"),
			waitUntil: ofcirv1.StateAvailable,

			expectedStates: []ofcirv1.CIResourceState{
				ofcirv1.StateNone, ofcirv1.StateProvisioning, ofcirv1.StateProvisioningWait, ofcirv1.StateAvailable,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			s := runtime.NewScheme()
			_ = ofcirv1.AddToScheme(s)
			_ = corev1.AddToScheme(s)

			cipool := tt.cipool.build()
			cir := tt.cir.pool(cipool).build()

			cipoolSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-secret", cipool.Name),
					Namespace: cipool.Namespace,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithRuntimeObjects(cipool, cipoolSecret, cir).Build()

			r := &CIResourceReconciler{
				Client: fakeClient,
			}

			cirKey := types.NamespacedName{
				Name:      cir.Name,
				Namespace: cir.Namespace,
			}

			var stateTransitions []ofcirv1.CIResourceState
			var latestErr error
			var latestCir ofcirv1.CIResource

			maxReconciles := 20
			counter := 0

			for counter = 0; counter < maxReconciles; counter++ {
				_, latestErr = r.Reconcile(context.TODO(), reconcile.Request{NamespacedName: cirKey})

				err := fakeClient.Get(context.TODO(), cirKey, &latestCir)
				assert.NoError(t, err)

				if len(stateTransitions) == 0 {
					stateTransitions = append(stateTransitions, latestCir.Status.State)
				} else if stateTransitions[len(stateTransitions)-1] != latestCir.Status.State {
					stateTransitions = append(stateTransitions, latestCir.Status.State)
				}

				if latestCir.Status.State == tt.waitUntil {
					break
				}
			}
			assert.Less(t, counter, maxReconciles, fmt.Sprintf("CIResource did not reach the required state. Waiting for state '%s', but got '%s'", tt.waitUntil, latestCir.Status.State))

			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, latestErr)
			} else {
				assert.NoError(t, latestErr)

				latestCir := ofcirv1.CIResource{}
				fakeClient.Get(context.TODO(), cirKey, &latestCir)
				assert.Equal(t, tt.expectedStates, stateTransitions)
			}
		})
	}
}

// cipoolBuilder allows to build a CIPool instance using a fluent interface
type cipoolBuilder struct {
	ofcirv1.CIPool
}

// cipool creates a new instance with useful default values
func cipool() *cipoolBuilder {
	return &cipoolBuilder{
		CIPool: ofcirv1.CIPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cipool-test",
				Namespace: defaultTestNs,
			},
			Spec: ofcirv1.CIPoolSpec{
				Provider: string(providers.ProviderDummy),
				Size:     10,
				Timeout: metav1.Duration{
					Duration: time.Hour * 4,
				},
				Priority: 0,
				State:    ofcirv1.StatePoolAvailable,
			},
		},
	}
}

const (
	defaultTestNs string = "test-ns"
)

func (cp *cipoolBuilder) build() *ofcirv1.CIPool {
	return &cp.CIPool
}

func (cp *cipoolBuilder) size(s int) *cipoolBuilder {
	cp.Spec.Size = s
	return cp
}

// cirBuilder allows to build a CIResource instance using a fluent interface
type cirBuilder struct {
	ofcirv1.CIResource
}

func cir(name string) *cirBuilder {
	return &cirBuilder{
		CIResource: ofcirv1.CIResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultTestNs,
			},
			Spec: ofcirv1.CIResourceSpec{
				State: ofcirv1.StateNone,
				Type:  ofcirv1.TypeCIHost,
				PoolRef: corev1.LocalObjectReference{
					Name: "",
				},
			},
			Status: ofcirv1.CIResourceStatus{
				State: ofcirv1.StateNone,
			},
		},
	}
}

func (cb *cirBuilder) build() *ofcirv1.CIResource {
	return &cb.CIResource
}

func (cb *cirBuilder) pool(p *ofcirv1.CIPool) *cirBuilder {
	cb.Spec.PoolRef.Name = p.Name
	return cb
}

func (cb *cirBuilder) currentState(s ofcirv1.CIResourceState) *cirBuilder {
	cb.Status.State = s
	return cb
}

func (cb *cirBuilder) requiredState(s ofcirv1.CIResourceState) *cirBuilder {
	cb.Spec.State = s
	return cb
}
