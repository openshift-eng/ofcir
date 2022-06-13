package providers

import (
	"fmt"
	"time"
)

const (
	maxAvailableResources = 10
)

type dummyInstance struct {
	Resource
	available bool
}
type Dummy struct {
	instances map[string]dummyInstance
}

func NewDummyProvider() Provider {
	dummy := &Dummy{
		instances: make(map[string]dummyInstance),
	}

	for n := 0; n < maxAvailableResources; n++ {
		instance := dummyInstance{
			Resource: Resource{
				Id:      fmt.Sprintf("dummy-%d", n),
				Address: fmt.Sprintf("1.1.1.%d", n),
			},
			available: true,
		}
		dummy.instances[instance.Id] = instance
	}

	return dummy
}

func (p *Dummy) Acquire() (Resource, error) {

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

func (p *Dummy) Status(id string) (Resource, error) {

	resource, ok := p.instances[id]
	if !ok {
		return Resource{}, fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	time.Sleep(time.Second * 1)
	return resource.Resource, nil
}

func (p *Dummy) Clean(id string) error {
	_, ok := p.instances[id]
	if !ok {
		return fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	time.Sleep(time.Second * 5)
	return nil
}

func (p *Dummy) Release(id string) error {
	resource, ok := p.instances[id]
	if !ok {
		return fmt.Errorf(fmt.Sprintf("Resource %s not found", id))
	}

	resource.available = true
	p.instances[id] = resource

	return nil
}
