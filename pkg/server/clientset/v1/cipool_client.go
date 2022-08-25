package v1

import (
	"context"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type CIPoolInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*ofcirv1.CIPoolList, error)
	Get(ctx context.Context, name string, options metav1.GetOptions) (*ofcirv1.CIPool, error)
}

type cipoolClient struct {
	restClient rest.Interface
	ns         string
}

func (c *cipoolClient) List(ctx context.Context, opts metav1.ListOptions) (*ofcirv1.CIPoolList, error) {
	result := ofcirv1.CIPoolList{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("cipools").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *cipoolClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*ofcirv1.CIPool, error) {
	result := ofcirv1.CIPool{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource("cipools").
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}
