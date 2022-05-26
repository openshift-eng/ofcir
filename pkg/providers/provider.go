package providers

import (
	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

type Provider interface {
	Clean(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error

	Acquire(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error

	Release(cir *ofcirv1.CIResource, cipool *ofcirv1.CIPool) error
}
