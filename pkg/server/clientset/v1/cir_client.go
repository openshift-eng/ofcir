package v1

import (
	"context"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

var (
	resourceName = "ciresources"
)

type CIResourceInterface interface {
	List(ctx context.Context, opts metav1.ListOptions) (*ofcirv1.CIResourceList, error)
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*ofcirv1.CIResource, error)
	Update(ctx context.Context, cir *ofcirv1.CIResource, opts metav1.UpdateOptions) (*ofcirv1.CIResource, error)
}

type cirClient struct {
	restClient rest.Interface
	ns         string
}

func (c *cirClient) List(ctx context.Context, opts metav1.ListOptions) (*ofcirv1.CIResourceList, error) {
	result := ofcirv1.CIResourceList{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource(resourceName).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *cirClient) Get(ctx context.Context, name string, opts metav1.GetOptions) (*ofcirv1.CIResource, error) {
	result := ofcirv1.CIResource{}
	err := c.restClient.
		Get().
		Namespace(c.ns).
		Resource(resourceName).
		VersionedParams(&opts, scheme.ParameterCodec).
		Do(ctx).
		Into(&result)

	return &result, err
}

func (c *cirClient) Update(ctx context.Context, cir *ofcirv1.CIResource, opts metav1.UpdateOptions) (*ofcirv1.CIResource, error) {
	result := ofcirv1.CIResource{}
	err := c.restClient.
		Put().
		Namespace(c.ns).
		Resource(resourceName).
		Name(cir.Name).
		VersionedParams(&opts, scheme.ParameterCodec).
		Body(cir).
		Do(ctx).
		Into(&result)

	return &result, err
}
