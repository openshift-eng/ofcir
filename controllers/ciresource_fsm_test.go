package controllers

import (
	"testing"
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestCIResourceFSMProcess(t *testing.T) {

	fakePool := &ofcirv1.CIPool{
		ObjectMeta: v1.ObjectMeta{
			Name: "fake-pool",
		},
		Spec: ofcirv1.CIPoolSpec{
			Provider: string(providers.ProviderDummy),
		},
	}

	tests := []struct {
		name               string
		cir                *ofcirv1.CIResource
		cipool             *ofcirv1.CIPool
		expectedIsDirty    bool
		expectedState      ofcirv1.CIResourceState
		expectedRetryAfter time.Duration
		expectedError      bool
	}{
		{
			name: "init",
			cir: &ofcirv1.CIResource{
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateNone,
				},
			},
			cipool:             fakePool,
			expectedIsDirty:    true,
			expectedState:      ofcirv1.StateProvisioning,
			expectedRetryAfter: defaultCirRetryDelay,
		},
		{
			name: "available->maintenance",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateMaintenance,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateAvailable,
				},
			},
			cipool:             fakePool,
			expectedIsDirty:    true,
			expectedState:      ofcirv1.StateMaintenance,
			expectedRetryAfter: defaultCirRetryDelay,
		},
		{
			name: "available->inuse",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateInUse,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateAvailable,
				},
			},
			cipool:             fakePool,
			expectedIsDirty:    true,
			expectedState:      ofcirv1.StateInUse,
			expectedRetryAfter: defaultCirRetryDelay,
		},
		{
			name: "maintenance->available",
			cir: &ofcirv1.CIResource{
				Spec: ofcirv1.CIResourceSpec{
					State: ofcirv1.StateAvailable,
				},
				Status: ofcirv1.CIResourceStatus{
					State: ofcirv1.StateMaintenance,
				},
			},
			cipool:             fakePool,
			expectedIsDirty:    true,
			expectedState:      ofcirv1.StateAvailable,
			expectedRetryAfter: defaultCirRetryDelay,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fsm := NewCIResourceFSM()
			isDirty, retryAfter, err := fsm.Process(tt.cir, tt.cipool)
			if !tt.expectedError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
			assert.Equal(t, tt.expectedIsDirty, isDirty)
			assert.Equal(t, tt.expectedState, tt.cir.Status.State)
			assert.Equal(t, tt.expectedRetryAfter, retryAfter)
		})
	}
}
