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
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/klient/k8s"
	"sigs.k8s.io/e2e-framework/klient/k8s/resources"
	"sigs.k8s.io/e2e-framework/klient/wait"
	"sigs.k8s.io/e2e-framework/klient/wait/conditions"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"
	"sigs.k8s.io/e2e-framework/pkg/envfuncs"
	"sigs.k8s.io/e2e-framework/support/kind"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

const (
	kindClusterName   = "ofcir-test"
	ofcirNamespace    = "ofcir-system"
	defaultOfcirImage = "localhost/ofcir-test:latest"
	ofcirImageArchive = "/tmp/ofcir-latest.tar"
)

var testenv env.Environment

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

func createKindCluster(clusterName string) env.Func {
	return envfuncs.CreateClusterWithConfig(kind.NewProvider(), clusterName, "kind-config.yaml")
}

func destroyKindCluster(clusterName string) env.Func {
	return envfuncs.DestroyCluster(clusterName)
}

func ofcirCleanup(ctx context.Context, c *envconf.Config) (context.Context, error) {
	log.Printf("Cleaning up")
	if err := os.Remove(ofcirImageArchive); err != nil && !os.IsNotExist(err) {
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

	log.Println("Waiting for ofcir operator to be ready")
	r := cfg.Client().Resources(ofcirNamespace)

	err := wait.For(
		conditions.New(r).ResourceListMatchN(&v1.PodList{}, 1, func(object k8s.Object) bool {
			pod := object.(*v1.Pod)
			for _, cond := range pod.Status.Conditions {
				if cond.Type == v1.PodReady && cond.Status == v1.ConditionTrue {
					return true
				}
			}
			return false
		}, resources.WithLabelSelector("control-plane=controller-manager")),
		wait.WithTimeout(180*time.Second),
		wait.WithInterval(5*time.Second),
	)
	if err != nil {
		var pods v1.PodList
		if listErr := r.List(context.Background(), &pods); listErr == nil {
			for _, pod := range pods.Items {
				log.Printf("Pod %s: phase=%s", pod.Name, pod.Status.Phase)
				for _, cs := range pod.Status.ContainerStatuses {
					log.Printf("  container %s: ready=%v state=%+v", cs.Name, cs.Ready, cs.State)
				}
			}
		}
		return ctx, fmt.Errorf("timed out waiting for ofcir operator to be ready: %w", err)
	}

	log.Println("Ofcir operator is ready")
	return ctx, nil
}

func runCommand(command string) *exec.Proc {
	return gexe.RunProc(command)
}
