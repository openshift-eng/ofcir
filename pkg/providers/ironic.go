package providers

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/baremetal/httpbasic"
	"github.com/gophercloud/gophercloud/openstack/baremetal/v1/nodes"
	"github.com/gophercloud/gophercloud/openstack/imageservice/v2/images"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/gophercloud/utils/openstack/clientconfig"
	"github.com/openshift/ofcir/pkg/utils"
	"go.etcd.io/etcd/pkg/transport"
)

var (
	clients        map[string]*gophercloud.ServiceClient = make(map[string]*gophercloud.ServiceClient)
	instance_infos map[string]map[string]string          = make(map[string]map[string]string)
	typeName       string                                = "ofcir_type"
	cirName        string                                = "ofcir_cir"
	cirTaken       string                                = "taken"
)

type ironicProviderConfig struct {
	Endpoint  string `json:"endpoint"`
	OSCloud   string `json:"oscloud"`
	CloudYAML string `json:"cloudyaml"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	Image     string `json:"image"`
	Sshkey    string `json:"sshkey"`
}
type ironicProvider struct {
	config        ironicProviderConfig
	client        *gophercloud.ServiceClient
	instance_info map[string]string
}

func IronicProviderFactory(providerInfo string, secretData map[string][]byte) (Provider, error) {
	config := ironicProviderConfig{
		Username:  "",
		Password:  "",
		OSCloud:   "openstack",
		CloudYAML: "",
		Endpoint:  "https://172.22.0.3:6385",
		Image:     "http://172.22.0.1/images/ofcir_image.qcow2",
	}

	if configJSON, ok := secretData["config"]; ok {
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return nil, fmt.Errorf("error in provider config json: %w", err)
		}
	}

	provider := &ironicProvider{
		config: config,
	}

	err := provider.UpdateClient(false)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

/* Create a new openstack baremetal client */
func (p *ironicProvider) UpdateClient(clearcache bool) error {
	var client *gophercloud.ServiceClient

	key := p.config.Endpoint + p.config.Username + p.config.OSCloud + p.config.CloudYAML + p.config.Image
	if clearcache {
		delete(clients, key)
	}

	// Assuming here that there is only a single reconcile loop,
	if _, ok := clients[key]; !ok {
		instance_info := make(map[string]string)
		if p.config.CloudYAML != "" {
			clouds_yaml_file := fmt.Sprintf("/tmp/%x.yaml", md5.Sum([]byte(key)))
			file, _ := os.Create(clouds_yaml_file)
			defer file.Close()
			decoded, _ := base64.StdEncoding.DecodeString(p.config.CloudYAML)
			file.Write(decoded)

			os.Setenv("OS_CLIENT_CONFIG_FILE", clouds_yaml_file)

			opts := new(clientconfig.ClientOpts)
			opts.Cloud = p.config.OSCloud
			client, _ = clientconfig.NewServiceClient("baremetal", opts)

			if !strings.Contains(p.config.Image, "://") {
				image_client, err := clientconfig.NewServiceClient("image", opts)
				image, err := images.Get(image_client, p.config.Image).Extract()
				if err != nil {
					return fmt.Errorf("Error getting image info: %w", err)
				}
				instance_info["image_source"] = image.ID

				// If there is no kernel or ramdisk then its a whole disk images
				// set the kernel and ramdisk to a blank string to ensure they are cleared
				kernel_id, k_ok := image.Properties["kernel_id"]
				ramdisk_id, r_ok := image.Properties["ramdisk_id"]
				if k_ok && r_ok {
					instance_info["kernel"] = kernel_id.(string)
					instance_info["ramdisk"] = ramdisk_id.(string)
				} else {
					instance_info["kernel"] = ""
					instance_info["ramdisk"] = ""
				}
			} else {
				instance_info["image_source"] = p.config.Image
				instance_info["image_checksum"] = p.config.Image + ".checksum"
			}

		} else {
			client, _ = httpbasic.NewBareMetalHTTPBasic(httpbasic.EndpointOpts{
				IronicEndpoint:     p.config.Endpoint,
				IronicUser:         p.config.Username,
				IronicUserPassword: p.config.Password,
			})
			instance_info["image_source"] = p.config.Image
			instance_info["image_checksum"] = p.config.Image + ".checksum"

			tlsInfo := transport.TLSInfo{
				InsecureSkipVerify: true,
			}
			tlsTransport, _ := transport.NewTransport(tlsInfo, time.Second*30)
			c := http.Client{
				Transport: tlsTransport,
			}
			client.HTTPClient = c
		}
		client.Microversion = "1.72"

		clients[key] = client
		instance_infos[key] = instance_info
	}
	p.client = clients[key]
	p.instance_info = instance_infos[key]
	return nil
}

func (p *ironicProvider) GetNode(id string) (*nodes.Node, error) {
	node, err := nodes.Get(p.client, id).Extract()
	// If the Auth token expired, log in and try again
	if err != nil && strings.Contains(err.Error(), "Authentication failed") {
		p.UpdateClient(true)
		node, err = nodes.Get(p.client, id).Extract()
	}
	return node, err
}

func (p *ironicProvider) UpdateNode(id string, opts nodes.UpdateOpts) (*nodes.Node, error) {
	node, err := nodes.Update(p.client, id, opts).Extract()
	// If the Auth token expired, log in and try again
	if err != nil && strings.Contains(err.Error(), "Authentication failed") {
		p.UpdateClient(true)
		node, err = nodes.Update(p.client, id, opts).Extract()
	}
	return node, err
}

func (p *ironicProvider) ChangeProvisionStateNode(id string, opts nodes.ProvisionStateOpts) error {
	err := nodes.ChangeProvisionState(p.client, id, opts).ExtractErr()
	// If the Auth token expired, log in and try again
	if err != nil && strings.Contains(err.Error(), "Authentication failed") {
		p.UpdateClient(true)
		err = nodes.ChangeProvisionState(p.client, id, opts).ExtractErr()
	}
	return err
}

func (p *ironicProvider) Acquire(poolSize int, poolName string, poolType string) (Resource, error) {
	res := Resource{}

	node, err := p.selectNode(poolType)
	if err != nil {
		return res, fmt.Errorf("error selecting node: %w", err)
	}

	res.Id = node.UUID
	return res, nil
}

func (p *ironicProvider) AcquireCompleted(id string) (bool, Resource, error) {
	node, err := p.GetNode(id)

	res := Resource{
		Id: id,
	}

	if err != nil {
		return false, res, fmt.Errorf("error getting node: %w", err)
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

	res.Address, _ = node.Extra["ofcir_ip"].(string)

	// Ironic can return this as a string or JSON
	if val, ok := node.Extra["ofcir_data"].(string); ok {
		res.Metadata = val
	} else {
		ofcir_data, _ := json.Marshal(node.Extra["ofcir_data"])
		res.Metadata = fmt.Sprintf("%s", ofcir_data)
	}

	sshport := "22"
	if v, found := node.Extra["ofcir_port_ssh"]; found {
		sshport = v.(string)
	}

	// Hold back on setting nodes to Available until ssh is available
	if !utils.IsPortOpen(res.Address, sshport) {
		return false, res, nil
	}
	return true, res, nil
}

func (p *ironicProvider) Clean(id string) error {
	node, err := p.GetNode(id)
	if err != nil {
		return fmt.Errorf("error getting node: %w", err)
	}
	return p.deployNode(*node)
}

func (p *ironicProvider) CleanCompleted(id string) (bool, error) {
	cleaned, _, err := p.AcquireCompleted(id)
	return cleaned, err
}

func (p *ironicProvider) Release(id string) error {
	node, _ := p.GetNode(id)
	node.Extra[cirName] = ""

	// We're clearing the cir metadata but not cleaning, in favour
	// of cleaning when the node is acquired
	p.UpdateNode(id, nodes.UpdateOpts{
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

	// Note: should be based on node.Properties["local_gb"]
	// but node.Properties appears to be empty
	// FS normally grows on first boot
	p.instance_info["root_gb"] = "64"

	p.UpdateNode(node.UUID, nodes.UpdateOpts{
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/extra",
			Value: node.Extra,
		},
		nodes.UpdateOperation{
			Op:    nodes.ReplaceOp,
			Path:  "/instance_info",
			Value: p.instance_info,
		},
	})

	newstate := nodes.TargetActive
	if node.ProvisionState == string(nodes.Active) {
		newstate = "rebuild"
	} else if node.ProvisionState == string(nodes.DeployFail) {
		newstate = "rebuild"
	}

	configDrive := map[string]interface{}{
		"meta_data": map[string]interface{}{
			"public_keys": map[string]interface{}{
				"0": p.config.Sshkey,
			},
		},
	}

	return p.ChangeProvisionStateNode(node.UUID,
		nodes.ProvisionStateOpts{
			Target:      newstate,
			ConfigDrive: configDrive,
		},
	)
}

// Note: If we ever increase the reconsole loop to run concurrent
// threads, some locking will be needed here
func (p *ironicProvider) selectNode(poolType string) (*nodes.Node, error) {
	var selectedNode *nodes.Node

	err := nodes.List(p.client, nodes.ListOpts{Fields: []string{"uuid,name,provision_state,extra"}}).EachPage(func(page pagination.Page) (bool, error) {
		thenodes, err := nodes.ExtractNodes(page)
		if err != nil {
			return false, fmt.Errorf("error extracting nodes: %w", err)
		}

		for x := 0; x < len(thenodes); x++ {
			node := thenodes[x]

			if node.ProvisionState != string(nodes.Active) && node.ProvisionState != string(nodes.Available) {
				continue
			}

			if node.Extra[typeName] == poolType && (node.Extra[cirName] == nil || node.Extra[cirName] == "") {
				selectedNode = &node
				err = p.deployNode(node)
				if err != nil {
					return false, fmt.Errorf("error deploying node: %w", err)
				}
				return false, nil
			}
		}
		return true, nil
	})

	// Problem connecting to ironic
	if err != nil {
		// Auth problems are expect each time the auth token times out (if using keystone)
		// re-auth and all should be ok the next time the reconcole loop runs
		if strings.Contains(err.Error(), "Authentication failed") {
			p.UpdateClient(true)
		}
		return nil, fmt.Errorf("error listing nodes: %w", err)
	}

	if selectedNode != nil {
		return selectedNode, nil
	}
	return nil, fmt.Errorf("Couldn't find a suitable ironic node")
}
