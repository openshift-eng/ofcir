package e2etests

import (
	"context"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"github.com/vladimirvivien/gexe"
	"github.com/vladimirvivien/gexe/exec"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/klient"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/support/kind"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

const (
	kindClusterName   = "ofcir-test"
	defaultOfcirImage = "localhost/ofcir-test:latest"
	ofcirImageArchive = "/tmp/ofcir-latest.tar"
)

var (
	testenv    env.Environment
	kubeconfig string
)

func TestMain(m *testing.M) {
	os.Setenv("GO_TEST_TIMEOUT", "10m")

	ofcirv1.AddToScheme(scheme.Scheme)

	testenv = env.New()

	testenv.Setup(
		createKindCluster(kindClusterName),
		buildAndLoadOfcirImage,
		deployOfcirOperator,
	)

	testenv.Finish(
		destroyKindCluster(kindClusterName),
		ofcirCleanup,
	)

	// launch package tests
	os.Exit(testenv.Run(m))
}

// This function is required because the current e2e-framework version (0.2.0) has a bug
// with podman, fixed in the latest version (see https://github.com/kubernetes-sigs/e2e-framework/pull/348).
// After upgrading ofcir dependencies, this function could be replaced by:
//
// envfuncs.CreateKindCluster(kindClusterName)
func createKindCluster(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Printf("Creating cluster (%s)", clusterName)

		var err error
		k := kind.NewCluster(clusterName)
		kubeconfig, err = k.CreateWithConfig("kindest/node:v1.30.0", "kind-config.yaml")
		if err != nil {
			return ctx, err
		}

		// Temporary fix for podman
		p := runCommand(fmt.Sprintf("sed -i '/enabling experimental podman provider/d' %s", kubeconfig))
		if p.Err() != nil {
			return ctx, fmt.Errorf("kind: create cluster failed: %s: %s", p.Err(), p.Result())
		}

		cfg.WithKubeconfigFile(kubeconfig)

		if err := waitForControlPlane(cfg.Client()); err != nil {
			return ctx, err
		}

		type kindContextKey string
		return context.WithValue(ctx, kindContextKey(clusterName), k), nil
	}
}

// This function is required because the current e2e-framework version (0.2.0) has a bug
// with podman, fixed in the latest version (see https://github.com/kubernetes-sigs/e2e-framework/pull/348).
// After upgrading ofcir dependencies, this function could be replaced by:
//
// envfuncs.DestroyKindCluster(kindClusterName)
func destroyKindCluster(clusterName string) env.Func {
	return func(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
		log.Printf("Destroying cluster (%s)", clusterName)

		p := runCommand(fmt.Sprintf(`kind delete cluster --name %s`, clusterName))
		if p.Err() != nil {
			return ctx, fmt.Errorf("kind: delete cluster failed: %s: %s", p.Err(), p.Result())
		}
		if err := os.RemoveAll(kubeconfig); err != nil {
			return ctx, fmt.Errorf("kind: remove kubefconfig failed: %w", err)
		}
		return ctx, nil
	}
}

func waitForControlPlane(client klient.Client) error {
	r, err := resources.New(client.RESTConfig())
	if err != nil {
		return err
	}
	selector, err := metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "component", Operator: metav1.LabelSelectorOpIn, Values: []string{"etcd", "kube-apiserver", "kube-controller-manager", "kube-scheduler"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 pods running the core apiserver components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	selector, err = metav1.LabelSelectorAsSelector(
		&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{
				{Key: "k8s-app", Operator: metav1.LabelSelectorOpIn, Values: []string{"kindnet", "kube-dns", "kube-proxy"}},
			},
		},
	)
	if err != nil {
		return err
	}
	// a kind cluster with one control-plane node will have 4 k8s-app pods running networking components
	err = wait.For(conditions.New(r).ResourceListN(&v1.PodList{}, 4, resources.WithLabelSelector(selector.String())))
	if err != nil {
		return err
	}
	return nil
}

func ofcirCleanup(ctx context.Context, c *envconf.Config) (context.Context, error) {
	log.Printf("Cleaning up")
	if err := os.Remove(ofcirImageArchive); err != nil {
		return ctx, err
	}
	return ctx, nil
}

func buildAndLoadOfcirImage(ctx context.Context, c *envconf.Config) (context.Context, error) {
	// OFCIR_IMAGE will be defined in CI, where the image is already built, so this step
	// can be skipped.
	if _, found := os.LookupEnv("OFCIR_IMAGE"); found {
		log.Printf("Skipping image build")
		return ctx, nil
	}

	ofcirImage := defaultOfcirImage
	log.Printf("Building ofcir image (%s)", ofcirImage)
	if p := gexe.New().SetEnv("IMG", ofcirImage).RunProc("make -C ../../ ofcir-image"); p.Err() != nil {
		log.Printf("Failed to build ofcir image: %s : %s", p.Out(), p.Err())
		return ctx, p.Err()
	}

	log.Println("Exporting ofcir image")
	if _, err := os.Stat(ofcirImageArchive); err == nil {
		os.Remove(ofcirImageArchive)
	}
	if p := runCommand(fmt.Sprintf("podman save -o %s %s", ofcirImageArchive, ofcirImage)); p.Err() != nil {
		log.Printf("Failed to export ofcir image: %s : %s", p.Out(), p.Err())
		return ctx, p.Err()
	}

	log.Println("Loading ofcir image into the cluster")
	cmd := fmt.Sprintf("kind load image-archive --name %s %s", kindClusterName, ofcirImageArchive)
	if p := runCommand(cmd); p.Err() != nil {
		log.Printf("Failed to export ofcir image: %s : %s", p.Out(), p.Err())
		return ctx, p.Err()
	}

	return ctx, nil
}

func deployOfcirOperator(ctx context.Context, cfg *envconf.Config) (context.Context, error) {
	log.Println("Deploying ofcir operator")

	// If present, let's reuse OFCIR_IMAGE as pullspec
	ofcirImage := defaultOfcirImage
	if val, found := os.LookupEnv("OFCIR_IMAGE"); found {
		ofcirImage = val
	}

	if p := gexe.New().SetEnv("IMG", ofcirImage).SetEnv("KUSTOMIZE_BUILD_DIR", "config/e2e").RunProc("make -C ../../ deploy"); p.Err() != nil {
		log.Printf("Failed to deploy ofcir operator: %s : %s", p.Out(), p.Err())
		return ctx, p.Err()
	}

	// Wait for ofcir operator ready
	time.Sleep(30 * time.Second)
	return ctx, nil
}

func runCommand(command string) *exec.Proc {
	return gexe.RunProc(command)
}
