package controllers

import (
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/openshift/ofcir/pkg/providers"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// cipoolBuilder allows to build a CIPool instance using a fluent interface
type cipoolBuilder struct {
	ofcirv1.CIPool
}

// cipool creates a new instance with useful default values
func cipool() *cipoolBuilder {
	return &cipoolBuilder{
		CIPool: ofcirv1.CIPool{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cipool-test",
				Namespace: defaultTestNs,
			},
			Spec: ofcirv1.CIPoolSpec{
				Provider: string(providers.ProviderDummy),
				Size:     10,
				Timeout: metav1.Duration{
					Duration: time.Hour * 4,
				},
				Priority: 0,
				State:    ofcirv1.StatePoolAvailable,
			},
			Status: ofcirv1.CIPoolStatus{},
		},
	}
}

func cipoolWithSecret() (*cipoolBuilder, *corev1.Secret) {
	cip := cipool()
	cipSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cip.Name,
			Namespace: cip.Namespace,
		},
	}
	return cip, cipSecret
}

const (
	defaultTestNs string = "test-ns"
)

func (cp *cipoolBuilder) build() *ofcirv1.CIPool {
	return &cp.CIPool
}

func (cp *cipoolBuilder) name(value string) *cipoolBuilder {
	cp.Name = value
	return cp
}

func (cp *cipoolBuilder) size(s int) *cipoolBuilder {
	cp.Spec.Size = s
	return cp
}

func (cp *cipoolBuilder) priority(value int) *cipoolBuilder {
	cp.Spec.Priority = value
	return cp
}

// cirBuilder allows to build a CIResource instance using a fluent interface
type cirBuilder struct {
	ofcirv1.CIResource
}

func cir(name string) *cirBuilder {
	return &cirBuilder{
		CIResource: ofcirv1.CIResource{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: defaultTestNs,
			},
			Spec: ofcirv1.CIResourceSpec{
				State: ofcirv1.StateNone,
				Type:  ofcirv1.TypeCIHost,
				PoolRef: corev1.LocalObjectReference{
					Name: "",
				},
			},
			Status: ofcirv1.CIResourceStatus{
				State: ofcirv1.StateNone,
			},
		},
	}
}

func (cb *cirBuilder) build() *ofcirv1.CIResource {
	return &cb.CIResource
}

func (cb *cirBuilder) pool(p string) *cirBuilder {
	cb.Spec.PoolRef.Name = p
	return cb
}

func (cb *cirBuilder) currentState(s ofcirv1.CIResourceState) *cirBuilder {
	cb.Status.State = s
	return cb
}

func (cb *cirBuilder) requiredState(s ofcirv1.CIResourceState) *cirBuilder {
	cb.Spec.State = s
	return cb
}
