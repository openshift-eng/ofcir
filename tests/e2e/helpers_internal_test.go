package e2etests

import (
	"context"
	"fmt"
	"os"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
)

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

func waitForPoolReady(t *testing.T, r *resources.Resources, poolName string) (*ofcirv1.CIPool, *ofcirv1.CIResourceList) {
	pool := ofcirv1.CIPool{
		ObjectMeta: v1.ObjectMeta{Name: poolName, Namespace: "ofcir-system"},
	}

	// Wait until pool reaches the required size
	err := wait.For(conditions.New(r).ResourceMatch(&pool, func(object k8s.Object) bool {
		p := object.(*ofcirv1.CIPool)
		return p.Status.Size == p.Spec.Size
	}))
	assert.NoError(t, err)

	// Wait until all of the pool resources become available
	var cirs ofcirv1.CIResourceList
	err = wait.For(conditions.New(r).ResourceListMatchN(&cirs, pool.Status.Size, func(object k8s.Object) bool {
		c := object.(*ofcirv1.CIResource)
		return c.Spec.PoolRef.Name == pool.Name && c.Status.State == ofcirv1.StateAvailable
	}))
	assert.NoError(t, err)

	return &pool, &cirs
}

func waitForPoolDelete(t *testing.T, r *resources.Resources, pool *ofcirv1.CIPool) {
	err := wait.For(conditions.New(r).ResourceDeleted(pool))
	assert.NoError(t, err)
}

func deletePool(t *testing.T, r *resources.Resources, pool *ofcirv1.CIPool) {
	err := r.Delete(context.Background(), pool)
	assert.NoError(t, err)
}

func waitForCIRsDelete(t *testing.T, r *resources.Resources, cirs *ofcirv1.CIResourceList) {
	err := wait.For(conditions.New(r).ResourcesDeleted(cirs))
	assert.NoError(t, err)
}
