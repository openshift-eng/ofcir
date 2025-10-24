package e2etests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

// OfcirClient is an helper class for interacting with the ofcir api component
// in the e2e tests
type OfcirClient struct {
	t       *testing.T
	r       *resources.Resources
	client  *http.Client
	baseUrl string
	token   string
}

func NewOfcirClient(t *testing.T, cfg *envconf.Config, token string) *OfcirClient {
	host := kindNodeHost(t, cfg)
	port := serviceNodePort(t, cfg, "ofcir-service", "http")

	return &OfcirClient{
		t:       t,
		r:       cfg.Client().Resources(ofcirNamespace),
		client:  &http.Client{},
		baseUrl: fmt.Sprintf("http://%s:%d", host, port),
		token:   token,
	}
}

type OfcirAcquire struct {
	Name         string
	Pool         string
	Provider     string
	ProviderInfo string
	Type         string
}

func (c *OfcirClient) doRequest(method string, commandUrl string) ([]byte, error) {
	destUrl := fmt.Sprintf("%s/%s", c.baseUrl, commandUrl)

	req, err := http.NewRequest(method, destUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("X-Ofcirtoken", c.token)

	r, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	if r.Status == "401 Unauthorized" {
		return nil, fmt.Errorf("401 Unauthorized")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s", body)
	}

	return body, nil
}

func (c *OfcirClient) Acquire(cirtype string) (*OfcirAcquire, error) {
	body, err := c.doRequest("POST", fmt.Sprintf("v1/ofcir?type=%s", cirtype))
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

func (c *OfcirClient) TryAcquire(cirtype string) *OfcirAcquire {
	acquire, err := c.Acquire(cirtype)
	require.NoError(c.t, err)
	return acquire
}

func (c *OfcirClient) TryAcquireCIR(cirtype string) *ofcirv1.CIResource {
	acquire, err := c.Acquire(cirtype)
	require.NoError(c.t, err)

	var cir ofcirv1.CIResource
	err = c.r.Get(context.Background(), acquire.Name, ofcirNamespace, &cir)
	require.NoError(c.t, err)

	return &cir
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
	body, err := c.doRequest("GET", fmt.Sprintf("v1/ofcir/%s", id))
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
	require.NoError(c.t, err)
	return status
}

func (c *OfcirClient) Release(id string) (string, error) {
	body, err := c.doRequest("DELETE", fmt.Sprintf("v1/ofcir/%s", id))
	if err != nil {
		return "", err
	}

	release := string(body)

	return release, nil
}

func (c *OfcirClient) TryRelease(id string) string {
	release, err := c.Release(id)
	require.NoError(c.t, err)
	return release
}

func (c *OfcirClient) TryReleaseCIR(cir *ofcirv1.CIResource) string {
	return c.TryRelease(cir.Name)
}
