package providers

import (
	"fmt"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	v1 "k8s.io/api/core/v1"
)

type ProviderType string

const (
	ProviderDummy    ProviderType = "fake-provider"
	ProviderLibvirt  ProviderType = "libvirt"
	ProviderIronic   ProviderType = "ironic"
	ProviderEquinix  ProviderType = "equinix"
	ProviderIbmcloud ProviderType = "ibmcloud"
)

func NewProvider(pool *ofcirv1.CIPool, poolSecret *v1.Secret) (Provider, error) {

	switch ProviderType(pool.Spec.Provider) {
	case ProviderDummy:
		return DummyProviderFactory(pool.Spec.ProviderInfo, poolSecret.Data), nil
	case ProviderLibvirt:
		return LibvirtProviderFactory(pool.Spec.ProviderInfo, poolSecret.Data)
	case ProviderIronic:
		return IronicProviderFactory(pool.Spec.ProviderInfo, poolSecret.Data)
	case ProviderEquinix:
		return EquinixProviderFactory(pool.Spec.ProviderInfo, poolSecret.Data)
	case ProviderIbmcloud:
		return IbmcloudProviderFactory(pool.Spec.ProviderInfo, poolSecret.Data)
	default:
		return nil, fmt.Errorf("unknown provider type: %s", pool.Spec.Provider)
	}
}
