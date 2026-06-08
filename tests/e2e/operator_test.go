package e2etests

import (
	"context"
	"testing"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/features"
)

func TestOperatorDeploymentWithoutKubeRBACProxy(t *testing.T) {
	testenv.Test(t, features.New("operator deployment").
		Assess("controller pod runs operator and api only", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			pod := getControllerPod(t, cfg)

			containerNames := make([]string, len(pod.Spec.Containers))
			for i, c := range pod.Spec.Containers {
				containerNames[i] = c.Name
			}
			assert.ElementsMatch(t, []string{"ofcir-operator", "ofcir-api"}, containerNames)
			for _, name := range containerNames {
				assert.NotEqual(t, "kube-rbac-proxy", name)
			}

			for _, cs := range pod.Status.ContainerStatuses {
				assert.True(t, cs.Ready, "container %s is not ready", cs.Name)
			}

			return ctx
		}).
		Assess("ofcir-service exposes only the API NodePort in e2e", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var service v1.Service
			err := cfg.Client().Resources().Get(ctx, "ofcir-service", ofcirNamespace, &service)
			require.NoError(t, err)
			require.Len(t, service.Spec.Ports, 1)
			assert.Equal(t, v1.ServiceTypeNodePort, service.Spec.Type)

			httpPort := service.Spec.Ports[0]
			assert.Equal(t, "http", httpPort.Name)
			assert.Equal(t, int32(8087), httpPort.TargetPort.IntVal)
			assert.Equal(t, int32(30007), httpPort.NodePort)

			return ctx
		}).
		Feature())
}

func TestAPILifecycleOverHTTP(t *testing.T) {
	testenv.Test(t, features.New("ofcir HTTP API lifecycle").
		Setup(ofcirSetup("pool-with-2-cirs", "pool-with-2-cirs")).
		Assess("status and release work without kube-rbac-proxy", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			r := cfg.Client().Resources(ofcirNamespace)
			c := NewOfcirClient(t, cfg, ctx.Value(tokenKey).(string))

			waitForPoolReady(t, r, "pool-with-2-cirs")

			cir := c.TryAcquireCIR("host")
			waitForCIRState(t, r, cir, ofcirv1.StateInUse)

			status := c.TryStatus(cir.Name)
			assert.Equal(t, cir.Name, status.Name)
			assert.Equal(t, "pool-with-2-cirs", status.Pool)
			assert.Equal(t, string(ofcirv1.StateInUse), status.Status)

			c.TryReleaseCIR(cir)
			waitForCIRState(t, r, cir, ofcirv1.StateAvailable)

			return ctx
		}).
		Teardown(ofcirTeardown()).
		Feature())
}

func getControllerPod(t *testing.T, cfg *envconf.Config) *v1.Pod {
	t.Helper()

	var pods v1.PodList
	r := cfg.Client().Resources(ofcirNamespace)
	err := r.List(
		context.Background(),
		&pods,
		resources.WithLabelSelector("control-plane=controller-manager"),
	)
	require.NoError(t, err)
	require.Len(t, pods.Items, 1)

	pod := &pods.Items[0]
	require.NotEmpty(t, pod.Status.PodIP)
	return pod
}

func TestDeploymentSpecMatchesPostProxyRemovalExpectations(t *testing.T) {
	testenv.Test(t, features.New("deployment spec").
		Assess("operator enables built-in metrics auth filter", func(ctx context.Context, t *testing.T, cfg *envconf.Config) context.Context {
			var deploy appsv1.Deployment
			err := cfg.Client().Resources().Get(ctx, "ofcir-controller-manager", ofcirNamespace, &deploy)
			require.NoError(t, err)

			operatorContainer := findContainer(t, &deploy, "ofcir-operator")
			assert.Contains(t, operatorContainer.Args, "--health-probe-bind-address=:8081")
			require.NotEmpty(t, operatorContainer.Ports)
			assert.Equal(t, int32(8443), operatorContainer.Ports[0].ContainerPort)

			apiContainer := findContainer(t, &deploy, "ofcir-api")
			require.NotEmpty(t, apiContainer.Ports)
			assert.Equal(t, int32(8087), apiContainer.Ports[0].ContainerPort)

			return ctx
		}).
		Feature())
}

func findContainer(t *testing.T, deploy *appsv1.Deployment, name string) v1.Container {
	t.Helper()
	for _, c := range deploy.Spec.Template.Spec.Containers {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("container %q not found in deployment", name)
	return v1.Container{}
}
