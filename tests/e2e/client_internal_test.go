package e2etests

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

type OfcirClient struct {
	t       *testing.T
	baseUrl string
}

func NewOfcirClient(t *testing.T, cfg *envconf.Config) *OfcirClient {

	rawUrl := cfg.Client().RESTConfig().Host
	u, err := url.Parse(rawUrl)
	assert.NoError(t, err)
	host, _, err := net.SplitHostPort(u.Host)
	assert.NoError(t, err)

	var service v1.Service
	err = cfg.Client().Resources().Get(context.Background(), "ofcir-service", "ofcir-system", &service)
	assert.NoError(t, err)

	if len(service.Spec.Ports) != 1 {
		t.Fatalf("found more than one port defined for the ofcir-service")
		return nil
	}
	port := service.Spec.Ports[0].NodePort

	return &OfcirClient{
		t:       t,
		baseUrl: fmt.Sprintf("http://%s:%d", host, port),
	}
}

type OfcirAcquire struct {
	Name         string
	Pool         string
	Provider     string
	ProviderInfo string
	Type         string
}

func (c *OfcirClient) Acquire() (*OfcirAcquire, error) {
	destUrl := fmt.Sprintf("%s/v1/ofcir?type=host", c.baseUrl)
	r, err := http.Post(destUrl, "", nil)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	acquire := &OfcirAcquire{}
	err = json.Unmarshal(body, acquire)
	if err != nil {
		return nil, err
	}

	return acquire, nil
}

func (c *OfcirClient) TryAcquire() *OfcirAcquire {
	acquire, err := c.Acquire()
	assert.NoError(c.t, err)
	return acquire
}

type OfcirStatus struct {
	Name         string
	Pool         string
	Provider     string
	ProviderInfo string
	Type         string
	Ip           string
	Extra        string
	Status       string
}

func (c *OfcirClient) Status(id string) (*OfcirStatus, error) {
	destUrl := fmt.Sprintf("%s/v1/ofcir/%s", c.baseUrl, id)
	r, err := http.Get(destUrl)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	status := &OfcirStatus{}
	err = json.Unmarshal(body, status)
	if err != nil {
		return nil, err
	}

	return status, nil
}

func (c *OfcirClient) TryStatus(id string) *OfcirStatus {
	status, err := c.Status(id)
	assert.NoError(c.t, err)
	return status
}

func (c *OfcirClient) Release(id string) (string, error) {
	destUrl := fmt.Sprintf("%s/v1/ofcir/%s", c.baseUrl, id)
	req, err := http.NewRequest("DELETE", destUrl, nil)
	if err != nil {
		return "", err
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer r.Body.Close()

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return "", err
	}

	release := string(body)

	return release, nil
}

func (c *OfcirClient) TryRelease(id string) string {
	release, err := c.Release(id)
	assert.NoError(c.t, err)
	return release
}
