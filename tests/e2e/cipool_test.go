package e2etests

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
)

var (
	defaultWaitTimeout = 15 * time.Second
	defaultWaitPoll    = 1 * time.Second
)

// Waits until the specified pool reached the desidered size and all of its resources
// are available
func waitUntilPoolReady(t *testing.T, cfg *envconf.Config, poolName string) {
	r := cfg.Client().Resources("ofcir-system")
	ctx := context.Background()

	var lastErr error
	for start := time.Now(); time.Since(start) < defaultWaitTimeout; time.Sleep(defaultWaitPoll) {
		// Get the pool
		var cipool ofcirv1.CIPool
		lastErr = r.Get(ctx, poolName, "ofcir-system", &cipool)
		if lastErr != nil {
			if errors.IsNotFound(lastErr) {
				continue
			}
			t.Error(lastErr.Error())
		}

		// Wait for pool to reach the desired size
		if cipool.Status.Size != cipool.Spec.Size {
			continue
		}

		// Wait for the resource to be available
		cirs := getCIPoolResources(ctx, r, cipool.Name)
		numCirsAvailable := 0
		for _, cir := range cirs {
			if cir.Status.State == ofcirv1.StateAvailable {
				numCirsAvailable++
			}
		}

		if numCirsAvailable == cipool.Spec.Size {
			return
		}
	}

	t.Logf("pool %s not ready", poolName)
	t.FailNow()
}

func getCIPoolResources(ctx context.Context, r *resources.Resources, poolName string) []ofcirv1.CIResource {

	cirs := []ofcirv1.CIResource{}

	var cirList ofcirv1.CIResourceList
	err := r.List(ctx, &cirList)
	if err != nil {
		return cirs
	}

	for _, cir := range cirList.Items {
		if cir.Spec.PoolRef.Name == poolName {
			cirs = append(cirs, cir)
		}
	}

	return cirs
}

func waitUntil(t *testing.T, cfg *envconf.Config, f func(context.Context, *resources.Resources) bool) {
	r := cfg.Client().Resources("ofcir-system")
	ctx := context.Background()

	for start := time.Now(); time.Since(start) < defaultWaitTimeout; time.Sleep(defaultWaitPoll) {
		if !f(ctx, r) {
			continue
		}

		return
	}

	t.Log("condition timeout")
	t.FailNow()
}

func TestCIPool(t *testing.T) {

	testdata := os.DirFS("testdata")

	f := features.New("pool").
		Setup(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r, err := resources.New(cfg.Client().RESTConfig())
			assert.NoError(t, err)

			err = decoder.DecodeEachFile(ctx, testdata, "default_pool.yaml", decoder.CreateHandler(r))
			assert.NoError(t, err)

			return ctx
		}).
		Assess("all pool resources are deleted", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			waitUntilPoolReady(t, cfg, "default")

			waitUntil(t, cfg, func(ctx context.Context, r *resources.Resources) bool {
				return len(getCIPoolResources(ctx, r, "default")) == 0
			})

			return ctx
		}).
		Teardown(func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			// r, err := resources.New(cfg.Client().RESTConfig())
			// assert.NoError(t, err)

			// err = decoder.DecodeEachFile(ctx, testdata, "only_fallback.yaml", decoder.DeleteHandler(r))
			// assert.NoError(t, err)
			return ctx
		}).Feature()

	testenv.Test(t, f)
}
