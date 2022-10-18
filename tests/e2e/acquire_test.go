package e2etests

import (
	"context"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestAcquire(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition").
		Setup(ofcirSetup("pool-with-2-cirs")).
		Assess("acquire one resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg)

			waitForPoolReady(t, r, "pool-with-2-cirs")

			cir := c.TryAcquireCIR()
			waitForCIRState(t, r, cir, ofcirv1.StateInUse)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestAcquireAllResources(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition").
		Setup(ofcirSetup("pool-with-2-cirs")).
		Assess("acquire one resource", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg)

			_, cirs := waitForPoolReady(t, r, "pool-with-2-cirs")

			// Try to acquire all the resources offered by the pool
			for range cirs.Items {
				c.TryAcquireCIR()
			}

			// Next acquire must fail
			_, err := c.Acquire()
			assert.ErrorContains(t, err, "No available resource found")

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestPoolsPriority(t *testing.T) {

	testenv.Test(t, features.New("resource acquisition with priority").
		Setup(ofcirSetup("three-pools")).
		Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources("ofcir-system")
			c := NewOfcirClient(t, cfg)

			waitForsPoolReady(t, r)

			// cirInfo := c.TryAcquireCIR()
			// assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)

			// cirInfo = c.TryAcquireCIR()
			// assert.Equal(t, "pool-1", cirInfo.Spec.PoolRef.Name)

			cirInfo := c.TryAcquireCIR()
			assert.Equal(t, "pool-2", cirInfo.Spec.PoolRef.Name)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}
