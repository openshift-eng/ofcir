package e2etests

import (
	"context"
	"testing"
	"time"

	"gotest.tools/v3/assert"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	kwait "k8s.io/apimachinery/pkg/util/wait"
)

func TestDeleteEmptyPool(t *testing.T) {

	testenv.Test(t, features.New("delete an empty pool").
		Setup(ofcirSetup("pool-empty")).
		Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources()
			pool, cirs := waitForPoolReady(t, r, "pool-empty")

			deletePool(t, r, pool)

			waitForCIRsDelete(t, r, cirs)
			waitForPoolDelete(t, r, pool)
			return ctx
		}).Feature(),
	)
}

func TestDeletePoolWithOnlyAvailableResources(t *testing.T) {

	testenv.Test(t, features.New("delete a pool with availabe cirs").
		Setup(ofcirSetup("pool-with-2-cirs")).
		Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources()
			pool, cirs := waitForPoolReady(t, r, "pool-with-2-cirs")

			deletePool(t, r, pool)

			waitForCIRsDelete(t, r, cirs)
			waitForPoolDelete(t, r, pool)
			return ctx
		}).Feature())
}

func TestDeletePoolWithResourcesInUse(t *testing.T) {

	testenv.Test(t, features.New("delete a pool with cirs in use").
		Setup(ofcirSetup("pool-with-2-cirs")).
		Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources()
			pool, cirs := waitForPoolReady(t, r, "pool-with-2-cirs")

			c := NewOfcirClient(t, cfg)
			cir := c.TryAcquire()

			deletePool(t, r, pool)

			// Wait for some time, to be sure that the pool doesn't get deleted
			err := wait.For(conditions.New(r).ResourceDeleted(pool), wait.WithTimeout(10*time.Second))
			assert.Equal(t, kwait.ErrWaitTimeout, err)

			c.TryRelease(cir.Name)

			waitForCIRsDelete(t, r, cirs)
			waitForPoolDelete(t, r, pool)
			return ctx
		}).Feature())
}
