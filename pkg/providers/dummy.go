package providers

import (
	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

type Dummy struct {
}

func NewDummyProvider() Provider {
	return &Dummy{}
}

func (p *Dummy) Clean(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error {
	return nil
}

func (p *Dummy) Acquire(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error {
	return nil
}

func (p *Dummy) Release(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error {
	return nil
}
