package e2etests

import (
	"context"
	"sync"
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

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

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

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

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
		Assess("acquire resources respecting pool priority", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolsReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)

			cirInfo = c.TryAcquireCIR("host")
			assert.Equal(t, "pool-1", cirInfo.Spec.PoolRef.Name)

			cirInfo = c.TryAcquireCIR("host")
			assert.Equal(t, "pool-2", cirInfo.Spec.PoolRef.Name)

			// Fallback resources go through several stages before settling on "in use",
			// so wait for the state to be stable across multiple consecutive polls.
			waitForCIRStateStable(t, r, cirInfo, ofcirv1.StateInUse)

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
			assert.ErrorContains(t, e, "401 Unauthorized")
			return ctx
		}).
		Assess("can only get cir from authorized pool", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolsReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)

			_, e := c.Acquire("host")
			assert.ErrorContains(t, e, "No available resource found of type [host]")
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
			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolsReady(t, r)

			cirInfo := c.TryAcquireCIR("host")
			assert.Equal(t, "pool-0", cirInfo.Spec.PoolRef.Name)
			return ctx
		}).
		Assess("blocks when \"host2\" not specified and \"host\" not available", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))
			_, e := c.Acquire("host")
			assert.ErrorContains(t, e, "No available resource found of type [host]")
			return ctx
		}).
		Assess("allows when \"host2\" specified and available", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))
			cirInfo := c.TryAcquireCIR("host,host2")
			assert.Equal(t, "pool-1", cirInfo.Spec.PoolRef.Name)
			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestAcquireAndCheckStatus(t *testing.T) {

	testenv.Test(t, features.New("resource status check").
		Setup(ofcirSetup("pool-with-2-cirs", "pool-with-2-cirs")).
		Assess("acquire a resource and verify its status via the API", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolReady(t, r, "pool-with-2-cirs")

			cir := c.TryAcquireCIR("host")
			waitForCIRState(t, r, cir, ofcirv1.StateInUse)

			status := c.TryStatus(cir.Name)

			assert.Equal(t, cir.Name, status.Name)
			assert.Equal(t, "pool-with-2-cirs", status.Pool)
			assert.Equal(t, "in use", status.Status)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}

func TestConcurrentAcquireAndRelease(t *testing.T) {

	testenv.Test(t, features.New("concurrent acquire and release lifecycle").
		Setup(ofcirSetup("pool-with-2-cirs", "pool-with-2-cirs")).
		Assess("concurrent acquires yield distinct resources that can be released", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolReady(t, r, "pool-with-2-cirs")

			results := make([]*OfcirAcquire, 2)
			var wg sync.WaitGroup
			for i := range 2 {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					acquire, err := c.Acquire("host")
					if err != nil {
						t.Errorf("concurrent acquire %d failed: %v", idx, err)
						return
					}
					results[idx] = acquire
				}(i)
			}
			wg.Wait()

			if results[0] == nil || results[1] == nil {
				t.Fatal("one or both concurrent acquires failed")
			}
			assert.NotEqual(t, results[0].Name, results[1].Name, "concurrent acquires returned the same resource")

			for _, a := range results {
				var cir ofcirv1.CIResource
				err := r.Get(context.Background(), a.Name, ofcirNamespace, &cir)
				assert.NoError(t, err)

				waitForCIRState(t, r, &cir, ofcirv1.StateInUse)

				c.TryRelease(a.Name)
				waitForCIRState(t, r, &cir, ofcirv1.StateAvailable)
			}

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
			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

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

func TestConcurrentLoadBurst(t *testing.T) {
	const numRequests = 20
	const maxResponseTime = 10 * time.Second

	testenv.Test(t, features.New("concurrent load burst").
		Setup(ofcirSetup("pool-load-test", "pool-load-test")).
		Assess("all responses arrive within timeout budget", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolReady(t, r, "pool-load-test")

			type result struct {
				acquire    *OfcirAcquire
				err        error
				statusCode int
				duration   time.Duration
			}

			results := make([]result, numRequests)
			var wg sync.WaitGroup
			for i := range numRequests {
				wg.Add(1)
				go func(idx int) {
					defer wg.Done()
					start := time.Now()
					acq, err := c.Acquire("host")
					results[idx] = result{
						acquire:  acq,
						err:      err,
						duration: time.Since(start),
					}
				}(i)
			}
			wg.Wait()

			var successes int
			var maxDur time.Duration
			for i, res := range results {
				if res.duration > maxDur {
					maxDur = res.duration
				}

				if res.err != nil {
					assert.NotContains(t, res.err.Error(), "connection",
						"request %d had connection error: %v", i, res.err)
					assert.NotContains(t, res.err.Error(), "EOF",
						"request %d had EOF error: %v", i, res.err)
				}

				assert.Less(t, res.duration, maxResponseTime,
					"request %d took %v (exceeds %v budget)", i, res.duration, maxResponseTime)

				if res.acquire != nil {
					successes++
				}

				t.Logf("request %02d: duration=%v acquired=%v err=%v",
					i, res.duration.Round(time.Millisecond), res.acquire != nil, res.err)
			}

			assert.LessOrEqual(t, successes, 4,
				"pool has 4 resources, cannot acquire more")
			assert.Greater(t, successes, 0,
				"at least one request should succeed")
			assert.Less(t, maxDur, maxResponseTime,
				"slowest request took %v, expected under %v", maxDur, maxResponseTime)

			t.Logf("Summary: %d/%d acquired, max duration: %v", successes, numRequests, maxDur)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature(),
	)
}
