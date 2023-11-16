package providers

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"github.com/packethost/packngo"
)

const (
	hostPrefix string = "ofcir"
)

var client *packngo.Client
var eqOnce sync.Once

type equinixProviderConfig struct {
	ProjectID string `json:"projectid"` //project id in equinix
	Token     string `json:"token"`     //token for authentication
	Metro     string `json:"metro"`     //server location
	Plan      string `json:"plan"`      //server size
	OS        string `json:"os"`        //OS to install
}

type equinixProvider struct {
	config equinixProviderConfig
	client *packngo.Client
}

func EquinixProviderFactory(providerInfo string, secretData map[string][]byte) (Provider, error) {
	config := equinixProviderConfig{
		ProjectID: "",
		Token:     "",
		Metro:     "da",
		Plan:      "c3.small.x86",
		OS:        "rocky_8",
	}

	if configJson, ok := secretData["config"]; ok {
		if err := json.Unmarshal(configJson, &config); err != nil {
			return nil, fmt.Errorf("error in provider config json: %w", err)
		}
	}

	eqOnce.Do(func() {
		client = packngo.NewClientWithAuth("packngo lib", config.Token, nil)
	})

	return &equinixProvider{
		config: config,
		client: client,
	}, nil
}

func (p *equinixProvider) Acquire(poolSize int, poolName string, poolType string) (Resource, error) {
	resource := Resource{}

	//check how many resources exist and compare to pool size spec
	//to prevent a bug creating infinite resources
	deviceList, _, err := p.client.Devices.List(p.config.ProjectID, nil)
	if err != nil {
		return resource, fmt.Errorf("Error getting devices list: %w", err)
	}
	count := 0
	for _, d := range deviceList {
		for _, t := range d.Tags {
			if t == poolName {
				count++
			}
		}
	}
	if count >= poolSize {
		return resource, fmt.Errorf("Refusing to create device, already have %d and pool size is %d", count, poolSize)
	}

	uniqueId := strings.Replace(uuid.New().String(), "-", "", -1)
	resourceName := fmt.Sprintf("%s-%s", hostPrefix, uniqueId)

	cr := packngo.DeviceCreateRequest{
		Hostname:  resourceName,
		Metro:     p.config.Metro,
		Plan:      p.config.Plan,
		OS:        p.config.OS,
		ProjectID: p.config.ProjectID,
		Tags:      []string{poolName},
	}

	device, _, err := p.client.Devices.Create(&cr)
	if err != nil {
		return resource, fmt.Errorf("error creating device: %w", err)
	}

	resource.Id = device.ID
	return resource, nil
}

func (p *equinixProvider) AcquireCompleted(id string) (bool, Resource, error) {
	resource := Resource{
		Id: id,
	}

	device, _, err := p.client.Devices.Get(id, nil)
	if err != nil {
		return false, resource, fmt.Errorf("error getting device: %w", err)
	}

	if device.State == "active" {
		resource.Address = device.GetNetworkInfo().PublicIPv4
		return true, resource, nil
	}

	if device.State == "failed" {
		return false, resource, fmt.Errorf("device %s failed", id)
	}

	return false, resource, nil
}

func (p *equinixProvider) Clean(id string) error {
	rf := packngo.DeviceReinstallFields{
		DeprovisionFast: true,
		OperatingSystem: p.config.OS,
	}

	if _, err := p.client.Devices.Reinstall(id, &rf); err != nil {
		return fmt.Errorf("error reinstalling device: %w", err)
	}

	return nil
}

func (p *equinixProvider) CleanCompleted(id string) (bool, error) {
	cleaned, _, err := p.AcquireCompleted(id)
	return cleaned, err
}

func (p *equinixProvider) Release(id string) error {
	if _, err := p.client.Devices.Delete(id, true); err != nil {
		return fmt.Errorf("error deleting device: %w", err)
	}
	return nil
}
