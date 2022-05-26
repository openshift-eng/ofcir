package providers

import (
	"fmt"
)

type ProviderType string

const (
	ProviderDummy ProviderType = "fake-provider"
)

func NewProvider(providerType string) (Provider, error) {

	switch ProviderType(providerType) {
	case ProviderDummy:
		return NewDummyProvider(), nil
	default:
		return nil, fmt.Errorf("unknown provider type: %s", providerType)
	}
}
