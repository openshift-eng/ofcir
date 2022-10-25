package e2etests

import (
	"os"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/e2e-framework/klient/conf"
	"sigs.k8s.io/e2e-framework/pkg/env"
	"sigs.k8s.io/e2e-framework/pkg/envconf"

	ofcirv1 "github.com/openshift/ofcir/api/v1"
)

var (
	testenv env.Environment
)

func TestMain(m *testing.M) {

	path := conf.ResolveKubeConfigFile()
	cfg := envconf.NewWithKubeConfig(path)
	testenv = env.NewWithConfig(cfg)

	ofcirv1.AddToScheme(scheme.Scheme)

	// launch package tests
	os.Exit(testenv.Run(m))
}
