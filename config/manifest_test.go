package config_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func projectRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filename))
}

func kustomizeBuild(t *testing.T, overlay string) string {
	t.Helper()

	root := projectRoot(t)
	kustomize := filepath.Join(root, "bin", "kustomize")
	if _, err := os.Stat(kustomize); err != nil {
		t.Skipf("kustomize binary not found at %s (run: make kustomize)", kustomize)
	}

	cmd := exec.Command(kustomize, "build", filepath.Join(root, overlay))
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("kustomize build %s: %v\n%s", overlay, err, out)
	}
	return string(out)
}

func TestDefaultManifestsExcludeKubeRBACProxy(t *testing.T) {
	manifests := kustomizeBuild(t, "config/default")

	if strings.Contains(manifests, "kube-rbac-proxy") {
		t.Fatal("default manifests must not reference kube-rbac-proxy")
	}
	if strings.Contains(manifests, "auth_proxy") {
		t.Fatal("default manifests must not reference auth_proxy RBAC resources")
	}

	assertContainsAll(t, manifests,
		"name: ofcir-operator",
		"name: ofcir-api",
		"targetPort: 8087",
		"containerPort: 8443",
		"containerPort: 8087",
		"--health-probe-bind-address=:8081",
	)
}

func TestE2EManifestsExposeAPIDirectly(t *testing.T) {
	manifests := kustomizeBuild(t, "config/e2e")

	if strings.Contains(manifests, "kube-rbac-proxy") {
		t.Fatal("e2e manifests must not reference kube-rbac-proxy")
	}

	assertContainsAll(t, manifests,
		"name: http",
		"targetPort: 8087",
		"nodePort: 30007",
	)
	if strings.Contains(manifests, "  - name: https\n    port: 8443") {
		t.Fatal("e2e Service must not expose metrics on a NodePort (not mapped in kind)")
	}
}

func assertContainsAll(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	for _, needle := range needles {
		if !strings.Contains(haystack, needle) {
			t.Fatalf("manifests missing %q", needle)
		}
	}
}
