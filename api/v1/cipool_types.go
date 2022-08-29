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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CIPoolState defines the states for the CIPool
type CIPoolState string

func (c CIPoolState) String() string {
	return string(c)
}

const (
	// Indicates that the pool is active and can be selected when
	// looking for an eligible resource
	StatePoolAvailable CIPoolState = "available"

	// Indicates that the pool is not active and cannot be selected when
	// looking for an eligible resource
	StatePoolOffline CIPoolState = "offline"
)

// CIPoolSpec defines the desired state of CIPool
type CIPoolSpec struct {
	// Identifies the kind of the pool
	Provider string `json:"provider"`

	// Used for selecting an eligible pool
	Priority int `json:"priority"`

	// Desired number of instances maintained by the current pool
	Size int `json:"size"`

	// Store any useful instance info specific to the current provider type
	// +optional
	ProviderInfo string `json:"providerInfo,omitempty"`

	// Specify how long a CIR instance will be allowed to remain in the inuse state
	Timeout metav1.Duration `json:"timeout"`

	// Required state of the pool
	State CIPoolState `json:"state"`
}

// CIPoolStatus defines the observed state of CIPool
type CIPoolStatus struct {
	// Current state of the pool
	State CIPoolState `json:"state"`

	// Current number of instances maintained by the current pool
	Size int `json:"size"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=cip
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Provider",type="string",JSONPath=".spec.provider",description="The provider used by the pool to manage the resources"
//+kubebuilder:printcolumn:name="Priority",type="integer",JSONPath=".spec.priority",description="The priority of the pool"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="The current state"
//+kubebuilder:printcolumn:name="Size",type="integer",JSONPath=".status.size",description="The current size of the pool"
//+kubebuilder:printcolumn:name="Req Size",type="integer",JSONPath=".spec.size",description="The requested size of the pool"

// CIPool is the Schema for the cipools API
type CIPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CIPoolSpec   `json:"spec,omitempty"`
	Status CIPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CIPoolList contains a list of CIPool
type CIPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CIPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CIPool{}, &CIPoolList{})
}
