package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

type OfcirV11Interface interface {
	CIPools(namespace string) CIPoolInterface
}

type OfcirV1Client struct {
	restClient rest.Interface
}

func NewForConfig(c *rest.Config) (*OfcirV1Client, error) {
	config := *c
	config.ContentConfig.GroupVersion = &schema.GroupVersion{Group: "ofcir.openshift", Version: "v1"}
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	config.UserAgent = rest.DefaultKubernetesUserAgent()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, err
	}

	return &OfcirV1Client{restClient: client}, nil
}

func (c *OfcirV1Client) CIPools(namespace string) CIPoolInterface {
	return &cipoolClient{
		restClient: c.restClient,
		ns:         namespace,
	}
}

func (c *OfcirV1Client) CIResources(namespace string) CIResourceInterface {
	return &cirClient{
		restClient: c.restClient,
		ns:         namespace,
	}
}
