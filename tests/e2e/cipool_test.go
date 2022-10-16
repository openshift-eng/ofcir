package e2etests

import (
	"context"
	"fmt"
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
func waitUntilPoolReady(t *testing.T, cfg *envconf.Config, poolName string) *ofcirv1.CIPool {
	r := cfg.Client().Resources("ofcir-system")
	ctx := context.Background()

	var cipool ofcirv1.CIPool
	var lastErr error

	for start := time.Now(); time.Since(start) < defaultWaitTimeout; time.Sleep(defaultWaitPoll) {
		// Get the pool
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
		cirs := getPoolResources(ctx, r, cipool.Name)
		numCirsAvailable := 0
		for _, cir := range cirs {
			if cir.Status.State == ofcirv1.StateAvailable {
				numCirsAvailable++
			}
		}

		if numCirsAvailable == cipool.Spec.Size {
			return &cipool
		}
	}

	t.Logf("pool %s not ready", poolName)
	t.FailNow()

	return nil
}

func getPool(t *testing.T, cfg *envconf.Config, poolName string) ofcirv1.CIPool {
	r := cfg.Client().Resources("ofcir-system")

	var pool ofcirv1.CIPool
	err := r.Get(context.Background(), poolName, "ofcir-system", &pool)

	assert.NoError(t, err)
	return pool
}

func deletePool(t *testing.T, cfg *envconf.Config, pool *ofcirv1.CIPool) {
	r := cfg.Client().Resources("ofcir-system")

	err := r.Delete(context.Background(), pool)

	assert.NoError(t, err)
}

func getPoolResources(ctx context.Context, r *resources.Resources, poolName string) []ofcirv1.CIResource {

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

func ofcirSetup(testDataFile string) func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, err := resources.New(cfg.Client().RESTConfig())
		assert.NoError(t, err)

		err = decoder.DecodeEachFile(ctx, os.DirFS("testdata"), fmt.Sprintf("%s.yaml", testDataFile), decoder.CreateHandler(r))
		assert.NoError(t, err)

		return ctx
	}
}

func ofcirTeardown(testDataFile string) func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r, err := resources.New(cfg.Client().RESTConfig())
		assert.NoError(t, err)

		err = decoder.DecodeEachFile(ctx, os.DirFS("testdata"), fmt.Sprintf("%s.yaml", testDataFile), decoder.DeleteHandler(r))
		assert.NoError(t, err)

		return ctx
	}
}

func TestCIPool(t *testing.T) {

	f := features.New("pool").
		Setup(ofcirSetup("pool-with-2-cirs")).
		Assess("delete pool", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			// Wait until all the pool resources become available
			pool := waitUntilPoolReady(t, cfg, "pool-with-2-cirs")

			// Delete the pool
			deletePool(t, cfg, pool)

			// Wait until all the resources are removed
			waitUntil(t, cfg, func(ctx context.Context, r *resources.Resources) bool {
				return len(getPoolResources(ctx, r, "pool-with-2-cirs")) == 0
			})

			// Wait until the pool is removed
			waitUntil(t, cfg, func(ctx context.Context, r *resources.Resources) bool {

				var pool ofcirv1.CIPool
				err := r.Get(context.Background(), "pool-with-2-cirs", "ofcir-system", &pool)
				return errors.IsNotFound(err)
			})

			return ctx
		}).Feature()

	testenv.Test(t, f)
}
