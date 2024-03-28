package providers

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/google/uuid"
	"github.com/softlayer/softlayer-go/datatypes"
	"github.com/softlayer/softlayer-go/services"
	"github.com/softlayer/softlayer-go/session"
	"github.com/softlayer/softlayer-go/sl"

	"github.com/openshift/ofcir/pkg/utils"
)

// Package 200 are the fast provisioning servers (40-60min provsion)
const IbmCloudFastProvisioningServer = 200

// Tags used to mark manual hosts
const manualTag = "ofcir-manual"
const takenTag = "ofcir-taken"

// TODO: Would love to find a way to list locations where the package is available but
// I can't for the life of me figure it out, need to revisit
// I've removed some of the more expensive locations from the list below
var locations = []string{"DALLAS09", "DALLAS10", "DALLAS12", "DALLAS13", "FRANKFURT", "FRANKFURT04", "FRANKFURT05", "LONDON02", "LONDON04",
	"LONDON05", "LONDON06", "MILAN", "MONTREAL", "PARIS", "MADRID02", "MADRID04", "MADRID05", "SANJOSE03", "SANJOSE04", "TORONTO",
	"TORONTO04", "TORONTO05", "WASHINGTON06", "WASHINGTON07"}

type ibmcloudProviderConfig struct {
	APIKey string `json:"apikey"`
	Sshkey string `json:"sshkey"`
	Preset string `json:"preset"`
	OS     string `json:"os"`
}

type ibmcloudProvider struct {
	config ibmcloudProviderConfig
	client *session.Session
}

func IbmcloudProviderFactory(providerInfo string, secretData map[string][]byte) (Provider, error) {
	config := ibmcloudProviderConfig{
		OS: "OS_CENTOS_STREAM_8_X_64_BIT",
	}

	if configJSON, ok := secretData["config"]; ok {
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return nil, fmt.Errorf("error in provider config json: %w", err)
		}
	}

	provider := &ibmcloudProvider{
		config: config,
	}

	provider.client = session.New("apikey", provider.config.APIKey)
	return provider, nil
}

func (p *ibmcloudProvider) getPackageItems(packageId int) (resp []datatypes.Product_Item) {
	var mask = "id, itemCategory, keyName,prices[id, hourlyRecurringFee, recurringFee, categories]"
	var service = services.GetProductPackageService(p.client)
	receipt, err := service.Id(packageId).Mask(mask).GetItems()
	if err != nil {
		return
	}

	return receipt
}

func (p *ibmcloudProvider) getStandardPrice(item datatypes.Product_Item) (resp datatypes.Product_Item_Price) {
	for _, itemPrice := range item.Prices {
		if itemPrice.LocationGroupId == nil {
			return itemPrice
		}
	}
	return datatypes.Product_Item_Price{}

}

func (p *ibmcloudProvider) getItemPriceList(packageId int, itemKeyNames []string) (resp []datatypes.Product_Item_Price) {

	items := p.getPackageItems(packageId)
	var prices []datatypes.Product_Item_Price

	for _, itemKeyName := range itemKeyNames {
		for _, item := range items {
			if (*item.KeyName) == itemKeyName {
				itemPrice := p.getStandardPrice(item)
				prices = append(prices, itemPrice)
				break
			}
		}
	}

	return prices
}

func (p *ibmcloudProvider) serverCreate(pool, presetname string) (string, error) {

	location := locations[rand.Intn(len(locations))]

	hostname := strings.Split(uuid.New().String(), "-")[0]
	domain := pool + ".ofcir"

	key, err := p.ensureSSHKey(p.config.Sshkey)
	if err != nil {
		return "", err
	}

	server := []datatypes.Hardware{
		{
			Hostname: sl.String(hostname),
			Domain:   sl.String(domain),
		},
	}

	// IBMC Servers have to be odered with a range of packages, this is a list of the
	// minimum required by hourly BM nodes e.g. 1_IP_ADDRESS vs 4_PUBLIC_IP_ADDRESSES
	// Get a list of available keys with
	// ibmcloud sl order item-list  BARE_METAL_SERVER
	required_items := []string{
		"REBOOT_KVM_OVER_IP",
		"UNLIMITED_SSL_VPN_USERS_1_PPTP_VPN_USER_PER_ACCOUNT",
		p.config.OS,
		"BANDWIDTH_0_GB_2",
		"1_GBPS_PUBLIC_PRIVATE_NETWORK_UPLINKS",
		"1_IP_ADDRESS",
	}
	// Build a skeleton SoftLayer_Product_Item_Price objects.
	prices := p.getItemPriceList(IbmCloudFastProvisioningServer, required_items)

	// We need to find the ID of the preset with the required name
	packageService := services.GetProductPackageService(p.client)

	// ibmcloud sl order preset-list  --prices BARE_METAL_SERVER
	// e.g. presetname == 1U_4210S_384GB_2X4TB_RAID_1
	// 1U_4210S_384GB_2X4TB_RAID_1, id = 1278
	presets, err := packageService.Id(IbmCloudFastProvisioningServer).GetActivePresets()
	presetId := -1
	for _, preset := range presets {
		if *preset.KeyName == presetname {
			presetId = *preset.Id
			break
		}
	}
	if presetId == -1 {
		return "", errors.New("Can't find preset with name " + presetname)
	}

	orderkeys := []datatypes.Container_Product_Order_SshKeys{
		{
			SshKeyIds: []int{*key},
		},
	}

	// Build a container_Product_Order object.
	orderTemplate := datatypes.Container_Product_Order{
		Quantity:         sl.Int(1),
		Location:         sl.String(location),
		PackageId:        sl.Int(IbmCloudFastProvisioningServer),
		PresetId:         sl.Int(presetId),
		Prices:           prices,
		Hardware:         server,
		UseHourlyPricing: sl.Bool(true),
		SshKeys:          orderkeys,
	}

	// Get SoftLayer_Product_Order service.
	service := services.GetProductOrderService(p.client)

	// Uncomment to "dry-run" the order before placing it
	//result, err := service.VerifyOrder(&orderTemplate)
	_, err = service.PlaceOrder(&orderTemplate, sl.Bool(false))
	if err != nil {
		return "", err
	}

	// Nodes don't appear in ibmcloud immediatly, Should we wait for the node to appear?
	// would prevent a bunch of node not found errors in the logs from acquirecomplete
	return hostname, nil
}

func (p *ibmcloudProvider) Acquire(poolSize int, poolName string, poolType string) (Resource, error) {

	res := Resource{}

	// The ibmprovider can either pick up a existing baremetal node or create a new(hourly) one
	// If a node exists with the tag "ofcir-manual" and without the tag "ofcir-taken" we assume
	// it if available to be used, if not we create a new hourly server
	node, err := p.getAvailableHost()

	if err != nil {
		return res, err
	}

	if node != nil {
		res.Id = *node.Hostname
		return res, nil
	}

	if p.config.Preset == "" {
		return res, errors.New("No suitable hosts found and Preset not set to create Hosts")
	}

	id, err := p.serverCreate(poolName, p.config.Preset)
	if err != nil {
		return res, err
	}

	res.Id = id
	return res, nil
}

func (p *ibmcloudProvider) getNodeByName(name string) (*datatypes.Hardware, error) {
	service := services.GetAccountService(p.client)
	nodes, err := service.Mask("id;hostname;primaryIpAddress;hardwareStatus;billingItem;hourlyBillingFlag;lastTransaction;tagReferences;billingItem[categoryCode,package[name,id]]").GetHardware()
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		if *node.Hostname == name {
			return &node, nil
		}
	}
	return nil, errors.New("Node node found")
}

func (p *ibmcloudProvider) ensureSSHKey(newkey string) (*int, error) {
	sshKeys, err := services.GetAccountService(p.client).GetSshKeys()
	if err != nil {
		return nil, err
	}

	for _, key := range sshKeys {
		if *key.Key == newkey {
			return key.Id, nil
		}
	}
	newKey := datatypes.Security_Ssh_Key{}
	newKey.Key = sl.String(newkey)
	// TODO: could do with something desciptive here (poolname if available)
	newKey.Label = sl.String(strings.Split(uuid.New().String(), "-")[0])
	newKey, err = services.GetSecuritySshKeyService(p.client).CreateObject(&newKey)

	if err != nil {
		return nil, err
	}
	return newKey.Id, nil
}

func (p *ibmcloudProvider) getAvailableHost() (*datatypes.Hardware, error) {
	service := services.GetAccountService(p.client)
	// Only return Hardware with one of the manual or take tags
	nodeFilter := fmt.Sprintf("{\"hardware\":{\"tagReferences\":{\"tag\": {\"name\":{\"operation\": \"in\", "+
		"\"options\": [{\"name\": \"data\", \"value\": [\"%s\", \"%s\"]}]}}}}}", manualTag, takenTag)
	nodes, err := service.Mask("id;hostname;hardwareStatus;tagReferences").Filter(nodeFilter).GetHardware()
	if err != nil {
		return nil, err
	}

	for _, node := range nodes {
		taken := false
		manual := false
		for _, tag := range node.TagReferences {
			if *tag.Tag.Name == takenTag {
				taken = true
			}
			if *tag.Tag.Name == manualTag {
				manual = true
			}

		}

		if manual == true && taken == false && *node.HardwareStatus.Status == "ACTIVE" {
			service := services.GetHardwareServerService(p.client)
			service.Id(*node.Id).SetTags(sl.String(manualTag + "," + takenTag))
			p.Clean(*node.Hostname)
			return &node, nil
		}
	}
	return nil, nil
}

func (p *ibmcloudProvider) releaseManualHost(node *datatypes.Hardware) error {
	service := services.GetHardwareServerService(p.client)
	_, err := service.Id(*node.Id).SetTags(sl.String(manualTag))
	return err
}

func (p *ibmcloudProvider) AcquireCompleted(id string) (bool, Resource, error) {

	res := Resource{
		Id: id,
	}

	node, err := p.getNodeByName(id)
	if err != nil {
		return false, res, err
	}

	// This may happen for a short period (20 seconds or so) after a node is initially created
	if node == nil {
		return false, res, fmt.Errorf("error getting node: %w", err)
	}

	// not ready yet
	if *node.HardwareStatus.Status != "ACTIVE" {
		return false, res, nil
	}

	// A node may be "ACTIVE" but for a short period after a new OS is being provisioned,
	// acquire hasn't completed it just hasn't started. Check the TransactionStatus, it should
	// be COMPLETE if the node has finished provisioning
	if *node.LastTransaction.TransactionStatus.Name != "COMPLETE" {
		return false, res, nil
	}

	// Hold back on setting nodes to Available until ssh is available
	if !utils.IsPortOpen(*node.PrimaryIpAddress, "22") {
		return false, res, nil
	}

	res.Address = *node.PrimaryIpAddress

	return true, res, nil
}

func (p *ibmcloudProvider) Clean(id string) error {
	node, err := p.getNodeByName(id)

	if node == nil {
		if err != nil {
			return err
		}
		return errors.New("Error getting node")
	}

	key, err := p.ensureSSHKey(p.config.Sshkey)
	if err != nil {
		return err
	}

	required_items := []string{
		p.config.OS,
	}
	prices := p.getItemPriceList(*node.BillingItem.Package.Id, required_items)

	service := services.GetHardwareServerService(p.client)
	config := datatypes.Container_Hardware_Server_Configuration{}
	config.SshKeyIds = []int{*key}
	config.ItemPrices = prices

	_, err = service.Id(*node.Id).ReloadOperatingSystem(sl.String("FORCE"), &config)
	if err != nil {
		return err
	}

	return nil
}

func (p *ibmcloudProvider) CleanCompleted(id string) (bool, error) {
	cleaned, _, err := p.AcquireCompleted(id)
	return cleaned, err
}

func (p *ibmcloudProvider) Release(id string) error {
	node, err := p.getNodeByName(id)

	if node == nil {
		if err != nil {
			return err
		}
		return errors.New("Error getting node")
	}

	// If this was a manual host, we don't cancel it we just unmark it as taken
	for _, tag := range node.TagReferences {
		if *tag.Tag.Name == manualTag {
			return p.releaseManualHost(node)
		}
	}

	service := services.GetBillingItemService(p.client)
	_, err = service.Id(*node.BillingItem.Id).CancelItem(sl.Bool(*node.HourlyBillingFlag), sl.Bool(true), sl.String("No longer needed"), sl.String("No longer needed"))
	if err != nil {
		return err
	}
	return nil
}
