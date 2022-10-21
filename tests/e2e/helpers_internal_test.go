package e2etests

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	apimachinerywait "k8s.io/apimachinery/pkg/util/wait"
)

// This file contains a number of helper functions capturing the most common actions useful for writing the e2e test
// Their usage it's recommended also for improving the readability of the e2e tests

func ofcirSetup(testDataFile string) func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r := cfg.Client().Resources("ofcir-system")

		err := decoder.DecodeEachFile(ctx, os.DirFS("testdata"), fmt.Sprintf("%s.yaml", testDataFile), decoder.CreateHandler(r))
		assert.NoError(t, err)

		return ctx
	}
}

// Removes all the pools and cirs in the ofcir namespaces
func ofcirTeardown() func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
	return func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
		r := cfg.Client().Resources("ofcir-system")
		c := NewOfcirClient(t, cfg)

		// Release any cir still in use
		var cirs ofcirv1.CIResourceList
		err := r.List(ctx, &cirs)
		assert.NoError(t, err)
		for _, cir := range cirs.Items {
			if cir.Status.State == ofcirv1.StateInUse {
				c.TryReleaseCIR(&cir)
			}
		}

		// Remove all the pools
		var pools ofcirv1.CIPoolList
		err = r.List(ctx, &pools)
		assert.NoError(t, err)

		for _, pool := range pools.Items {
			deletePool(t, r, &pool)
			waitForPoolDelete(t, r, &pool)
		}

		// Verify that the environment is really clean
		err = r.List(ctx, &cirs)
		assert.NoError(t, err)
		assert.Len(t, cirs.Items, 0, "Found some dangling CIResources in the ofcir-system namespace")

		return ctx
	}
}

func waitForsPoolReady(t *testing.T, r *resources.Resources) (*ofcirv1.CIPoolList, *ofcirv1.CIResourceList) {

	var pools ofcirv1.CIPoolList
	err := r.List(context.Background(), &pools)
	assert.NoError(t, err)

	// Wait until all the pools are ready
	waitFor(t, conditions.New(r).ResourcesMatch(&pools, func(object k8s.Object) bool {
		p := object.(*ofcirv1.CIPool)
		return p.Status.Size == p.Spec.Size
	}))

	totalCirs := 0
	for _, p := range pools.Items {
		totalCirs += p.Spec.Size
	}

	// Wait until all of the cirs are available
	var cirs ofcirv1.CIResourceList
	waitFor(t, conditions.New(r).ResourceListMatchN(&cirs, totalCirs, func(object k8s.Object) bool {
		c := object.(*ofcirv1.CIResource)
		return c.Status.State == ofcirv1.StateAvailable
	}))

	return &pools, &cirs
}

func waitForPoolReady(t *testing.T, r *resources.Resources, poolName string) (*ofcirv1.CIPool, *ofcirv1.CIResourceList) {
	pool := ofcirv1.CIPool{
		ObjectMeta: v1.ObjectMeta{Name: poolName, Namespace: "ofcir-system"},
	}

	// Wait until pool reaches the required size
	waitFor(t, conditions.New(r).ResourceMatch(&pool, func(object k8s.Object) bool {
		p := object.(*ofcirv1.CIPool)
		return p.Status.Size == p.Spec.Size
	}))

	// Wait until all of the pool resources become available
	var cirs ofcirv1.CIResourceList
	waitFor(t, conditions.New(r).ResourceListMatchN(&cirs, pool.Status.Size, func(object k8s.Object) bool {
		c := object.(*ofcirv1.CIResource)
		return c.Spec.PoolRef.Name == pool.Name && c.Status.State == ofcirv1.StateAvailable
	}))

	return &pool, &cirs
}

func waitForPoolDelete(t *testing.T, r *resources.Resources, pool *ofcirv1.CIPool) {
	waitFor(t, conditions.New(r).ResourceDeleted(pool))
}

func deletePool(t *testing.T, r *resources.Resources, pool *ofcirv1.CIPool) {
	err := r.Delete(context.Background(), pool)
	assert.NoError(t, err)
}

func waitForCIRsDelete(t *testing.T, r *resources.Resources, cirs *ofcirv1.CIResourceList) {
	waitFor(t, conditions.New(r).ResourcesDeleted(cirs))
}

func waitForCIRState(t *testing.T, r *resources.Resources, cir *ofcirv1.CIResource, expectedState ofcirv1.CIResourceState) {
	waitFor(t, conditions.New(r).ResourceMatch(cir, func(object k8s.Object) bool {
		c := object.(*ofcirv1.CIResource)
		return c.Status.State == expectedState
	}))
}

func waitFor(t *testing.T, conditionFunc apimachinerywait.ConditionFunc, seconds ...int) {
	err := _waitFor(conditionFunc, seconds...)
	assert.NoError(t, err)
}

func waitNotFor(t *testing.T, conditionFunc apimachinerywait.ConditionFunc, seconds ...int) {
	err := _waitFor(conditionFunc, seconds...)
	assert.Equal(t, apimachinerywait.ErrWaitTimeout, err)
}

func _waitFor(conditionFunc apimachinerywait.ConditionFunc, seconds ...int) error {
	timeout := 180 * time.Second
	if len(seconds) > 0 {
		timeout = time.Duration(seconds[0]) * time.Second
	}

	return wait.For(conditionFunc, wait.WithImmediate(), wait.WithInterval(1*time.Second), wait.WithTimeout(timeout))
}
