package e2etests

import (
	"context"
	"fmt"
	"os"
	"testing"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/e2e-framework/klient/decoder"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
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

func waitForPoolReady(cfg *envconf.Config, poolName string) (*ofcirv1.CIPool, *ofcirv1.CIResourceList) {
	pool := ofcirv1.CIPool{
		ObjectMeta: v1.ObjectMeta{Name: poolName, Namespace: "ofcir-system"},
	}

	// Wait until pool reaches the required size
	wait.For(conditions.New(cfg.Client().Resources()).ResourceMatch(&pool, func(object k8s.Object) bool {
		p := object.(*ofcirv1.CIPool)
		return p.Status.Size == p.Spec.Size
	}))

	// Wait until all of the pool resources become available
	var cirs ofcirv1.CIResourceList
	wait.For(conditions.New(cfg.Client().Resources()).ResourceListMatchN(&cirs, pool.Status.Size, func(object k8s.Object) bool {
		c := object.(*ofcirv1.CIResource)
		return c.Spec.PoolRef.Name == pool.Name && c.Status.State == ofcirv1.StateAvailable
	}))

	return &pool, &cirs
}

func TestDeleteEmptyPool(t *testing.T) {

	testenv.Test(t,
		features.New("delete an empty pool").
			Setup(ofcirSetup("pool-empty")).
			Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

				r := cfg.Client().Resources()
				pool, cirs := waitForPoolReady(cfg, "pool-empty")

				r.Delete(context.Background(), pool)

				wait.For(conditions.New(r).ResourcesDeleted(cirs))
				wait.For(conditions.New(r).ResourceDeleted(pool))

				return ctx
			}).Feature(),
	)
}

func TestDeletePoolWithOnlyAvailableResources(t *testing.T) {

	testenv.Test(t,
		features.New("delete a pool with availabe cirs").
			Setup(ofcirSetup("pool-with-2-cirs")).
			Assess("", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {

				r := cfg.Client().Resources()
				pool, cirs := waitForPoolReady(cfg, "pool-with-2-cirs")

				r.Delete(context.Background(), pool)

				wait.For(conditions.New(r).ResourcesDeleted(cirs))
				wait.For(conditions.New(r).ResourceDeleted(pool))

				return ctx
			}).Feature())
}
