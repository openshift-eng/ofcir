package e2etests

import (
	"context"
	"fmt"
	"testing"
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestAcquire(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition").
		Setup(ofcirSetup("pool-with-2-cirs", "pool-with-2-cirs")).
		Assess("acquire one resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			waitForPoolReady(t, r, "pool-with-2-cirs")

			cir := c.TryAcquireCIR("host")
			waitForCIRState(t, r, cir, ofcirv1.StateInUse)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestAcquireAllResources(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition").
		Setup(ofcirSetup("pool-with-2-cirs", "pool-with-2-cirs")).
		Assess("acquire all resources", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			_, cirs := waitForPoolReady(t, r, "pool-with-2-cirs")

			// Try to acquire all the resources offered by the pool
			for range cirs.Items {
				c.TryAcquireCIR("host")
			}

			// Next acquire must fail
			_, err := c.Acquire("host")
			assert.ErrorContains(t, err, "No available resource found of type [host]")

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestPoolsPriority(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition with priority").
		Setup(ofcirSetup("three-pools", "pool-0,pool-1,pool-2")).
		Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			waitForsPoolReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)

			cirInfo = c.TryAcquireCIR("host")
			assert.Equal(t, "pool-1", cirInfo.Spec.PoolRef.Name)

			cirInfo = c.TryAcquireCIR("host")
			assert.Equal(t, "pool-2", cirInfo.Spec.PoolRef.Name)

			// Fallback resources quickly go through several stages before settling down on "in use"
			// Wait for things to settle down before so it hits TearDown in a inuse state
			// waitForCIRState isn't enough because the CIR appears to hit "in use" twice
			// TODO: Fix in state machine
			time.Sleep(time.Second * 3)
			waitForCIRState(t, r, cirInfo, ofcirv1.StateInUse)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestPoolsToken(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition by token").
		Setup(ofcirSetup("three-pools", "pool-0")).
		Assess("blocks when empty token", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			c := NewOfcirClient(t, cfg, "")
			_, e := c.Acquire("host")
			if assert.Error(t, e) {
				assert.Equal(t, fmt.Errorf("%q", "401 Unauthorized"), e)
			}
			return ctx
		}).
		Assess("can only get cir from authorized pool", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			waitForsPoolReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)

			_, e := c.Acquire("host")
			if assert.Error(t, e) {
				assert.Equal(t, fmt.Errorf("%q", "No available resource found of type [host]"), e)
			}
			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestPoolsTypes(t *testing.T) {
	testenv.Test(t, features.New("resource acquisition by type list").
		Setup(ofcirSetup("pools-different-types", "pool-0,pool-1")).
		Assess("allows when \"host\" available", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			waitForsPoolReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)
			return ctx
		}).
		Assess("blocks when \"host2\" not specified and \"host\" not available", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))
			_, e := c.Acquire("host")
			if assert.Error(t, e) {
				assert.Equal(t, fmt.Errorf("%q", "No available resource found of type [host]"), e)
			}
			return ctx
		}).
		Assess("allows when \"host2\" specified and available", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))
			cirInfo := c.TryAcquireCIR("host,host2")
			assert.Equal(t, "pool-1", cirInfo.Spec.PoolRef.Name)
			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestAcquireDurationResources(t *testing.T) {
	testenv.Test(t, features.New("resource acquisition").
		Setup(ofcirSetup("pool-duration", "pool-duration")).
		Assess("acquire a resources with short duration", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg, ctx.Value("token").(string))

			waitForPoolReady(t, r, "pool-duration")

			cir := c.TryAcquireCIR("host")

			waitForCIRState(t, r, cir, ofcirv1.StateInUse)
			// CIR should be released after 10 seconds (actually a minute due to reconcile loop timing)
			waitForCIRState(t, r, cir, ofcirv1.StateAvailable)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}
