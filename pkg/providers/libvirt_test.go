package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"libvirt.org/go/libvirt"
)

func TestAcquire(t *testing.T) {
	t.Skip()

	p := LibvirtProviderFactory("", map[string][]byte{})
	res, err := p.Acquire()
	assert.NoError(t, err)

	var isReady bool
	for i := 0; i < 20; i++ {
		isReady, res, err = p.AcquireCompleted(res.Id)
		assert.NoError(t, err)
		if err != nil {
			break
		}

		if isReady {
			break
		}

		time.Sleep(5 * time.Second)
	}

	assert.True(t, isReady)
	assert.NotEmpty(t, res.Address)
}

func TestRelease(t *testing.T) {
	t.Skip()

	p := LibvirtProviderFactory("", map[string][]byte{})

	conn, _ := libvirt.NewConnect("qemu:///system")
	defer conn.Close()

	domains, err := conn.ListAllDomains(libvirt.CONNECT_LIST_DOMAINS_ACTIVE)
	assert.NoError(t, err)

	_ = time.Now()

	for _, d := range domains {
		name, err := d.GetName()
		assert.NoError(t, err)

		if strings.HasPrefix(name, "ofcir-vm") {
			err := p.Release(name)
			assert.NoError(t, err)
		}
	}
}

func TestClean(t *testing.T) {
	t.Skip()

	p := LibvirtProviderFactory("", map[string][]byte{})

	conn, _ := libvirt.NewConnect("qemu:///system")
	defer conn.Close()

	res, err := p.Acquire()
	assert.NoError(t, err)

	time.Sleep(1 * time.Minute)

	err = p.Clean(res.Id)
	assert.NoError(t, err)
}

// func TestProviderInfoConfiguration(t *testing.T) {

// 	jsonConfig := `{
// "pool_name": "default"
// "volume_capacity": 20
// "backing_store": "/images/fedora-coreos-36.20220806.3.0-qemu.x86_64.qcow2"
// "memory": 2
// "cpus": 2
// "brdige": "virbr0"
// "ignitionPath": "/ignition/coreos.ign"
// }`

// 	p, err := LibvirtProviderFactory(jsonConfig, map[string][]byte{})
// 	assert.NoError(t, err)
// }
