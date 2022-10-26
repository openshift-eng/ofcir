package providers

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"libvirt.org/go/libvirt"
)

var (
	secretData = map[string][]byte{
		"config": []byte(`{
"pool": "default",
"volume": 20,
"backing_store": "fedora-coreos-36.20220806.3.0-qemu.x86_64.qcow2",
"memory": 4,
"cpus": 2,
"bridge": "virbr0",
"ignition": "coreos.ign"
}`)}
)

func TestAcquire(t *testing.T) {
	t.Skip()

	p, err := LibvirtProviderFactory("", secretData)
	assert.NoError(t, err)
	res, err := p.Acquire(1, "test")
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

	p, _ := LibvirtProviderFactory("", map[string][]byte{})

	conn, _ := libvirt.NewConnect("qemu:///system")
	defer conn.Close()

	domains, err := conn.ListAllDomains(0)
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

	p, _ := LibvirtProviderFactory("", map[string][]byte{})

	conn, _ := libvirt.NewConnect("qemu:///system")
	defer conn.Close()

	res, err := p.Acquire(1, "test")
	assert.NoError(t, err)

	time.Sleep(1 * time.Minute)

	err = p.Clean(res.Id)
	assert.NoError(t, err)
}

func TestMetatdata(t *testing.T) {

	// conn, _ := libvirt.NewConnect("qemu:///system")
	// defer conn.Close()

	// domain, err := conn.LookupDomainByName("ofcir-vm-e4439cc63d464089a8229ba122602c01")
	// assert.NoError(t, err)

	// conn.domain
}
