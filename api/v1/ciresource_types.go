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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CIResourceState defines the states for the CIResource
type CIResourceState string

func (c CIResourceState) String() string {
	return string(c)
}

const (
	// StateNone means the state is unknown
	StateNone CIResourceState = ""

	// StateProvisioning is the initial state when a new instance is allocated
	StateProvisioning CIResourceState = "provisioning"

	// StateProvisioningWait waits for the instance to be provisioned
	StateProvisioningWait CIResourceState = "provisioning wait"

	// StateAvailable is when an instance is ready to be picked up to host a CI job
	StateAvailable CIResourceState = "available"

	// StateInUse defines when an instance is currently running a CI job
	StateInUse CIResourceState = "in use"

	// StateMaintenance indicates that the instance exists and is defined, but it
	// cannot be picked up to host CI jobs
	StateMaintenance CIResourceState = "maintenance"

	// StateCleaning indicates the instance is getting cleaned, so that it
	// could be used for the next incoming requests
	StateCleaning CIResourceState = "cleaning"

	// StateCleaningWait waits for the instance to be cleaned
	StateCleaningWait CIResourceState = "cleaning wait"

	// StateDelete manages the removal of the resource
	StateDelete CIResourceState = "delete"

	// StateError is a terminal state in case something wrong happens during
	// the provisioning or the cleaning state
	StateError CIResourceState = "error"
)

const (
	EvictionLabel string = "ofcir/eviction"

	CIResourceFinalizer string = "ofcir.openshift/finalizer"
)

// CIResourceType defines the possible types for a CIResource
type CIResourceType string

const (
	// Default type to identify a single host
	TypeCIHost CIResourceType = "host"

	// A set of instances defining a cluster
	TypeCICluster CIResourceType = "cluster"
)

// CIResourceSpec defines the desired state of CIResource
type CIResourceSpec struct {
	// Reference to the CIPool that is managing the current CIResource
	PoolRef corev1.LocalObjectReference `json:"poolRef"`

	// The desired state for the CIResource
	State CIResourceState `json:"state"`

	// Additional information to support clusters
	Extra string `json:"extra"`

	// The type of the current resource
	Type CIResourceType `json:"type"`
}

// CIResourceStatus defines the observed state of CIResource
type CIResourceStatus struct {
	// The unique identifier of the resource currently requested
	ResourceId string `json:"resourceId"`

	// Public IPv4 address
	Address string `json:"address"`

	// Store any useful instance info specific to the current provider type
	// +optional
	ProviderInfo string `json:"providerInfo,omitempty"`

	// Current state of the resource
	State CIResourceState `json:"state"`

	// LastUpdated identifies when this status was last observed.
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:resource:shortName=cir
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Address",type="string",JSONPath=".status.address",description="Public IPv4 address"
//+kubebuilder:printcolumn:name="State",type="string",JSONPath=".status.state",description="Current state"
//+kubebuilder:printcolumn:name="Pool",type="string",JSONPath=".spec.poolRef.name",description="Pool owning the current resource"
//+kubebuilder:printcolumn:name="Res Id",type="string",JSONPath=".status.resourceId",description="Resource Id"

// CIResource represents a physical allocated instance (or set of instances) from a specific pool
type CIResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CIResourceSpec   `json:"spec,omitempty"`
	Status CIResourceStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// CIResourceList contains a list of CIResource
type CIResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CIResource `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CIResource{}, &CIResourceList{})
}
