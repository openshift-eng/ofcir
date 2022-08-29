package providers

import (
	"fmt"
	"sync"
	"time"
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

func (p *dummyProvider) Acquire() (Resource, error) {

	for _, i := range p.instances {
		if i.available {
			i.available = false
			p.instances[i.Id] = i
			time.Sleep(time.Second * 2)
			return i.Resource, nil
		}
	}

	return Resource{}, fmt.Errorf("no available resources found")
}

func (p *dummyProvider) AcquireCompleted(id string) (bool, Resource, error) {

	resource, ok := p.instances[id]
	if !ok {
		return false, Resource{}, fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	time.Sleep(time.Second * 5)

	return true, resource.Resource, nil
}

func (p *dummyProvider) Clean(id string) error {
	_, ok := p.instances[id]
	if !ok {
		return fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	time.Sleep(time.Second * 2)
	return nil
}

func (p *dummyProvider) CleanCompleted(id string) (bool, error) {
	_, ok := p.instances[id]
	if !ok {
		return false, fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	time.Sleep(time.Second * 3)
	return true, nil
}

func (p *dummyProvider) Release(id string) error {
	resource, ok := p.instances[id]
	if !ok {
		return fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	resource.available = true
	p.instances[id] = resource

	return nil
}
