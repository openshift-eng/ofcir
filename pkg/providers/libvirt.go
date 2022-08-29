package providers

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"libvirt.org/go/libvirt"
	"libvirt.org/go/libvirtxml"
)

var (
	libvirtDomainPrefix = "ofcir-vm"
	libvirtImagesPath   = "/var/lib/libvirt/images"
)

type libvirtProvider struct {
	poolName     string //pool used for the volume
	volCapacity  uint64 //volume capacity (GiB)
	backingStore string //backing store used for the vm, must be qcow2
	memory       uint   //amount of memory (GiB)
	cpus         uint   //number of vcpus
	bridge       string //the name of the bridge to be used
	ignitionPath string //absolute ignition file path
}

func LibvirtProviderFactory(providerInfo string, secretData map[string][]byte) Provider {
	l := &libvirtProvider{
		poolName:     "default",
		volCapacity:  20,
		backingStore: "/ofcir/tests/fedora-coreos-36.20220806.3.0-qemu.x86_64.qcow2",
		memory:       2,
		cpus:         2,
		bridge:       "virbr0",
		ignitionPath: "/ofcir/tests/coreos.ign",
	}
	return l
}

func (p *libvirtProvider) Acquire() (Resource, error) {

	uniqueId := strings.Replace(uuid.New().String(), "-", "", -1)
	resourceName := fmt.Sprintf("%s-%s", libvirtDomainPrefix, uniqueId)

	res := Resource{
		Id: resourceName,
	}

	if err := p.createVM(res.Id, ""); err != nil {
		return res, err
	}

	return res, nil
}

func (p *libvirtProvider) AcquireCompleted(id string) (bool, Resource, error) {

	res := Resource{
		Id:      id,
		Address: "",
	}

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return false, res, err
	}
	defer conn.Close()

	domain, err := conn.LookupDomainByName(id)
	if err != nil {
		return false, res, err
	}

	// Check if domain is active
	isActive, err := domain.IsActive()
	if err != nil {
		return false, res, err
	}
	if !isActive {
		return false, res, nil
	}

	// Look for IP Address
	network, err := conn.LookupNetworkByName(p.poolName)
	if err != nil {
		return false, res, err
	}

	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return false, res, err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return false, res, err
	}

	macAddress := domcfg.Devices.Interfaces[0].MAC.Address

	leases, err := network.GetDHCPLeases()
	if err != nil {
		return false, res, err
	}

	for _, l := range leases {
		if l.Mac == macAddress {
			res.Address = l.IPaddr
			return true, res, nil
		}
	}

	return false, res, nil
}

func (p *libvirtProvider) Clean(id string) error {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Get current domain MAC
	domain, err := conn.LookupDomainByName(id)
	if err != nil {
		return err
	}

	xmldoc, err := domain.GetXMLDesc(0)
	if err != nil {
		return err
	}

	domcfg := &libvirtxml.Domain{}
	err = domcfg.Unmarshal(xmldoc)
	if err != nil {
		return err
	}

	macAddress := domcfg.Devices.Interfaces[0].MAC.Address

	// Destroy the vm
	err = p.destroyVM(id)
	if err != nil {
		return err
	}

	// Recreate the vm with the same name and mac
	err = p.createVM(id, macAddress)
	if err != nil {
		return err
	}

	return nil
}

func (p *libvirtProvider) CleanCompleted(id string) (bool, error) {
	return true, nil
}

func (p *libvirtProvider) Release(id string) error {
	return p.destroyVM(id)
}

func (p *libvirtProvider) createVM(name string, macAddress string) error {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return err
	}
	defer conn.Close()

	//Check if vm already exists
	_, err = conn.LookupDomainByName(name)
	if err == nil {
		return fmt.Errorf("domain %s already exists", name)
	}

	lverr, ok := err.(libvirt.Error)
	if ok && lverr.Code != libvirt.ERR_NO_DOMAIN {
		return err
	}

	// Create volume
	pool, err := conn.LookupStoragePoolByName(p.poolName)
	if err != nil {
		return err
	}

	volCfg := &libvirtxml.StorageVolume{
		Type: "file",
		Name: fmt.Sprintf("%s.qcow2", name),
		Capacity: &libvirtxml.StorageVolumeSize{
			Value: p.volCapacity,
			Unit:  "GiB",
		},
		Allocation: &libvirtxml.StorageVolumeSize{
			Value: 51318784,
			Unit:  "bytes",
		},
		Target: &libvirtxml.StorageVolumeTarget{
			Path: fmt.Sprintf("%s/%s.qcow2", libvirtImagesPath, name),
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
			Permissions: &libvirtxml.StorageVolumeTargetPermissions{
				Mode:  "0600",
				Owner: "0",
				Group: "0",
				Label: "system_u:object_r:virt_image_t:s0",
			},
		},
		BackingStore: &libvirtxml.StorageVolumeBackingStore{
			Path: p.backingStore,
			Format: &libvirtxml.StorageVolumeTargetFormat{
				Type: "qcow2",
			},
		},
	}

	volDoc, err := volCfg.Marshal()
	if err != nil {
		return err
	}

	_, err = pool.StorageVolCreateXML(volDoc, 0)
	if err != nil {
		return err
	}

	// Create virtual machine
	domInterface := libvirtxml.DomainInterface{
		Source: &libvirtxml.DomainInterfaceSource{
			Bridge: &libvirtxml.DomainInterfaceSourceBridge{
				Bridge: p.bridge,
			},
		},
		Model: &libvirtxml.DomainInterfaceModel{
			Type: "virtio",
		},
	}
	if macAddress != "" {
		domInterface.MAC = &libvirtxml.DomainInterfaceMAC{
			Address: macAddress,
		}
	}

	domCfg := &libvirtxml.Domain{
		Type: "kvm",
		Name: name,
		Metadata: &libvirtxml.DomainMetadata{
			XML: "<libosinfo:libosinfo xmlns:libosinfo=\"http://libosinfo.org/xmlns/libvirt/domain/1.0\"><libosinfo:os id=\"http://fedoraproject.org/coreos/stable\"/></libosinfo:libosinfo>",
		},
		Memory: &libvirtxml.DomainMemory{
			Value: p.memory,
			Unit:  "GiB",
		},
		CurrentMemory: &libvirtxml.DomainCurrentMemory{
			Value: p.memory,
			Unit:  "GiB",
		},
		VCPU: &libvirtxml.DomainVCPU{
			Value: p.cpus,
		},
		OS: &libvirtxml.DomainOS{
			Type: &libvirtxml.DomainOSType{
				Arch:    "x86_64",
				Machine: "q35",
				Type:    "hvm",
			},
			BootDevices: []libvirtxml.DomainBootDevice{
				{
					Dev: "hd",
				},
			},
		},
		Features: &libvirtxml.DomainFeatureList{
			ACPI: &libvirtxml.DomainFeature{},
			APIC: &libvirtxml.DomainFeatureAPIC{},
		},
		CPU: &libvirtxml.DomainCPU{
			Mode: "host-passthrough",
		},
		Clock: &libvirtxml.DomainClock{
			Offset: "utc",
			Timer: []libvirtxml.DomainTimer{
				{
					Name:       "rtc",
					TickPolicy: "catchup",
				},
				{
					Name:       "pit",
					TickPolicy: "delay",
				},
				{
					Name:    "hpet",
					Present: "no",
				},
			},
		},
		PM: &libvirtxml.DomainPM{
			SuspendToMem: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
			SuspendToDisk: &libvirtxml.DomainPMPolicy{
				Enabled: "no",
			},
		},
		Devices: &libvirtxml.DomainDeviceList{
			Emulator: "/usr/bin/qemu-system-x86_64",
			Disks: []libvirtxml.DomainDisk{
				{
					Driver: &libvirtxml.DomainDiskDriver{
						Name:    "qemu",
						Type:    "qcow2",
						Discard: "unmap",
					},
					Source: &libvirtxml.DomainDiskSource{
						File: &libvirtxml.DomainDiskSourceFile{
							File: fmt.Sprintf("%s/%s.qcow2", libvirtImagesPath, name),
						},
					},
					Target: &libvirtxml.DomainDiskTarget{
						Dev: "vda",
						Bus: "virtio",
					},
				},
			},
			Interfaces: []libvirtxml.DomainInterface{
				domInterface,
			},
			Consoles: []libvirtxml.DomainConsole{
				{
					TTY: "pty",
				},
			},
		},
		QEMUCommandline: &libvirtxml.DomainQEMUCommandline{
			Args: []libvirtxml.DomainQEMUCommandlineArg{
				{
					Value: "-fw_cfg",
				},
				{
					Value: fmt.Sprintf("name=opt/com.coreos/config,file=%s", p.ignitionPath),
				},
			},
		},
	}

	xmlDoc, err := domCfg.Marshal()
	if err != nil {
		return err
	}

	domain, err := conn.DomainDefineXML(xmlDoc)
	if err != nil {
		return err
	}

	err = domain.Create()
	if err != nil {
		return err
	}

	return nil
}

func (p *libvirtProvider) destroyVM(id string) error {

	conn, err := libvirt.NewConnect("qemu:///system")
	if err != nil {
		return err
	}
	defer conn.Close()

	// Delete domain
	domain, err := conn.LookupDomainByName(id)
	if err != nil {
		lverr, ok := err.(libvirt.Error)
		if ok && lverr.Code == libvirt.ERR_NO_DOMAIN {
			return NewResourceNotFoundError(id)
		}

		return err
	}

	running, err := domain.IsActive()
	if err != nil {
		return err
	}

	if running {
		err = domain.Destroy()
		if err != nil {
			return err
		}
	}

	err = domain.Undefine()
	if err != nil {
		return err
	}

	// Delete domain volume
	key := fmt.Sprintf("%s/%s.qcow2", libvirtImagesPath, id)
	vol, err := conn.LookupStorageVolByKey(key)
	if err == nil {
		err = vol.Delete(libvirt.STORAGE_VOL_DELETE_NORMAL)
		if err != nil {
			return err
		}
	}

	return nil
}
