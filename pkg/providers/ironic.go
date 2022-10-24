package providers

import (
	"encoding/json"
	"fmt"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/pagination"
	"go.etcd.io/etcd/pkg/transport"
	"net/http"
	"time"
)

var (
	clients  map[string]*gophercloud.ServiceClient = make(map[string]*gophercloud.ServiceClient)
	typeName string                                = "type"
	cirName  string                                = "cir"
	cirTaken string                                = "taken"
)

type ironicProviderConfig struct {
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
	Image    string `json:"image"`
}
type ironicProvider struct {
	config ironicProviderConfig
	client *gophercloud.ServiceClient
}

func IronicProviderFactory(providerInfo string, secretData map[string][]byte) (Provider, error) {

	config := ironicProviderConfig{
		Username: "",
		Password: "",
		Endpoint: "https://172.22.0.3:6385",
		Image:    "http://172.22.0.1/images/ofcir_image.qcow2",
	}

	if configJSON, ok := secretData["config"]; ok {
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return nil, fmt.Errorf("error in provider config json: %w", err)
		}
	}

	key := config.Endpoint + config.Username
	// Assuming here that there is only a single reconcile loop,
	if _, ok := clients[key]; !ok {
		client, _ := httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
			IronicEndpoint:     config.Endpoint,
			IronicUser:         config.Username,
			IronicUserPassword: config.Password,
		})
		clients[key] = client
	}
	client := clients[key]

	tlsInfo := transport.TLSInfo{
		InsecureSkipVerify: true,
	}
	tlsTransport, _ := transport.NewTransport(tlsInfo, time.Second*30)
	c := http.Client{
		Transport: tlsTransport,
	}

	client.HTTPClient = c
	client.Microversion = "1.74"

	return &ironicProvider{
		config: config,
		client: client,
	}, nil
}

func (p *ironicProvider) Acquire() (Resource, error) {
	res := Resource{}

	node, err := p.selectNode()
	if err != nil {
		return res, fmt.Errorf("error selecting node: %w", err)
	}

	res.Id = node.UUID
	return res, nil
}

func (p *ironicProvider) AcquireCompleted(id string) (bool, Resource, error) {
	node, err := nodes.Get(p.client, id).Extract()

	res := Resource{
		Id: id,
	}

	if err != nil {
		return false, res, fmt.Errorf("error getting node: %w",err)
	}

	if node.ProvisionState == string(nodes.DeployFail) {
		err := p.deployNode(*node)
		if err != nil {
			return false, res, fmt.Errorf("error deploying node: %w", err)
		}
	}

	if node.ProvisionState != string(nodes.Active) {
		return false, res, nil
	}

	res.Address, _ = node.Extra["ip"].(string)
	return true, res, nil
}

func (p *ironicProvider) Clean(id string) error {
	node, err := nodes.Get(p.client, id).Extract()
	if err != nil {
		return fmt.Errorf("error getting node: %w",err)
	}
	return p.deployNode(*node)
}

func (p *ironicProvider) CleanCompleted(id string) (bool, error) {
	cleaned, _, err := p.AcquireCompleted(id)
	return cleaned, err
}

func (p *ironicProvider) Release(id string) error {
	node, _ := nodes.Get(p.client, id).Extract()
	node.Extra[cirName] = ""

	// We're clearing the cir metadata but not cleaning, in favour
	// of cleaning when the node is acquired
	nodes.Update(p.client, id, nodes.UpdateOpts{
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/extra",
			Value: node.Extra,
		},
	})
	return nil
}

func (p *ironicProvider) deployNode(node nodes.Node) error {
	node.Extra[cirName] = cirTaken
	instance_info := make(map[string]string)
	instance_info["image_source"] = p.config.Image
	instance_info["image_checksum"] = p.config.Image + ".checksum"
	nodes.Update(p.client, node.UUID, nodes.UpdateOpts{
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/extra",
			Value: node.Extra,
		},
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/instance_info",
			Value: instance_info,
		},
	})

	// TODO: needs configdrive
	newstate := nodes.TargetActive
	if node.ProvisionState == string(nodes.Active) {
		newstate = "rebuild"
	} else if node.ProvisionState == string(nodes.DeployFail) {
		newstate = "rebuild"
	}
	return nodes.ChangeProvisionState(p.client, node.UUID,
		nodes.ProvisionStateOpts{
			Target: newstate,
		},
	).ExtractErr()
}

// Note: If we ever increase the reconsole loop to run concurrent
// threads, some locking will be needed here
func (p *ironicProvider) selectNode() (*nodes.Node, error) {
	var selectedNode *nodes.Node

	err := nodes.List(p.client, nodes.ListOpts{Fields: []string{"uuid,name,provision_state,extra"}}).EachPage(func(page pagination.Page) (bool, error) {
		thenodes, err := nodes.ExtractNodes(page)
		if err != nil {
			return false, fmt.Errorf("error extracting nodes: %w", err)
		}

		for x := 0; x < len(thenodes); x++ {
			node := thenodes[x]
			// Todo: type should be part of the CIR
			if node.Extra[typeName] == "host" && (node.Extra[cirName] == nil || node.Extra[cirName] == "") {
				selectedNode = &node
				p.deployNode(node)
				return false, nil
			}
		}
		return true, nil
	})

	// Problem connecting to ironic
	if err != nil {
		return nil, fmt.Errorf("error listing nodes: %w", err)
	}

	if selectedNode != nil {
		return selectedNode, nil
	}
	return nil, fmt.Errorf("Couldn't find a suitable ironic node")
}
