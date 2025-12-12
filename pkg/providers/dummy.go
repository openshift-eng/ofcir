package providers

import (
	"fmt"
	"sync"
)

const (
	maxAvailableResources = 10
)

type dummyInstance struct {
	Resource
	available bool
}
type dummyProvider struct {
	instances map[string]dummyInstance
}

var dummy *dummyProvider
var once sync.Once

func DummyProviderFactory(providerInfo string, secretData map[string][]byte) Provider {
	once.Do(func() {
		dummy = &dummyProvider{
			instances: make(map[string]dummyInstance),
		}

		for n := 0; n < maxAvailableResources; n++ {
			instance := dummyInstance{
				Resource: Resource{
					Id:       fmt.Sprintf("dummy-%d", n),
					Address:  fmt.Sprintf("1.1.1.%d", n),
					Metadata: "{}",
				},
				available: true,
			}
			dummy.instances[instance.Id] = instance
		}
	})

	return dummy
}

func (p *dummyProvider) Acquire(poolSize int, poolName string, poolType string) (Resource, error) {

	for _, i := range p.instances {
		if i.available {
			i.available = false
			p.instances[i.Id] = i
			return i.Resource, nil
		}
	}

	return Resource{}, fmt.Errorf("no available resources found")
}

func (p *dummyProvider) AcquireCompleted(id string) (bool, Resource, error) {

	resource, ok := p.instances[id]
	if !ok {
		return false, Resource{}, fmt.Errorf("resource %s not found", id)
	}

	return true, resource.Resource, nil
}

func (p *dummyProvider) Clean(id string) error {
	_, ok := p.instances[id]
	if !ok {
		return fmt.Errorf("resource %s not found", id)
	}

	return nil
}

func (p *dummyProvider) CleanCompleted(id string) (bool, error) {
	_, ok := p.instances[id]
	if !ok {
		return false, fmt.Errorf("resource %s not found", id)
	}

	return true, nil
}

func (p *dummyProvider) Release(id string) error {
	resource, ok := p.instances[id]
	if !ok {
		return fmt.Errorf("resource %s not found", id)
	}

	resource.available = true
	p.instances[id] = resource

	return nil
}
