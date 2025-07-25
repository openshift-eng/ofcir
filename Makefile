
# Image URL to use all building/pushing image targets
IMG ?= localhost/ofcir:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23
# KUSTOMIZE_BUILD_DIR defines the root folder to be used for manifests generation
KUSTOMIZE_BUILD_DIR ?= config/default


# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: generate-mocks controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

.PHONY: docs
docs: ## Generate the doc assests
	dot -Tpng docs/cir-states.dot -o docs/cir-states.png

.PHONY: generate-mocks
generate-mocks: mockgen
	find . -name 'mock_*.go' -type f -not -path './vendor/*' -delete
	PATH=$(LOCALBIN):$$PATH go generate -v $(shell go list ./...)

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	CGO_ENABLED=1 go build -o bin/ofcir-operator main.go
	CGO_ENABLED=0 go build -o bin/ofcir-api cmd/ofcir-api/main.go

.PHONY: unit-tests
unit-tests: fmt vet
	go test ./controllers/... ./pkg/...

.PHONY: e2e-tests
e2e-tests: 
	go test -v ./tests/e2e/...

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./main.go

.PHONY: ofcir-image
ofcir-image: 
	podman build -t ${IMG} -f Dockerfile .

##@ Deployment

## Location for storing the manifests to be deployed in the target cluster
DEPLOY_MANIFESTS_DIR ?= $(shell pwd)/ofcir-manifests
$(DEPLOY_MANIFESTS_DIR):
	mkdir -p $(DEPLOY_MANIFESTS_DIR)


ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image ofcir-operator-image=${IMG}
	$(KUSTOMIZE) build ${KUSTOMIZE_BUILD_DIR} | kubectl apply -f -

.PHONY: generate-deploy-manifests ## Same as deploy, but the output is stored into $DEPLOY_MANIFESTS_DIR
generate-deploy-manifests: $(DEPLOY_MANIFESTS_DIR) manifests kustomize 
	cd config/manager && $(KUSTOMIZE) edit set image ofcir-operator-image=${IMG}
	$(KUSTOMIZE) build ${KUSTOMIZE_BUILD_DIR} > $(DEPLOY_MANIFESTS_DIR)/ofcir-operator.yaml

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build ${KUSTOMIZE_BUILD_DIR} | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: test-deploy
test-deploy: generate-deploy-manifests
	minikube image build -t ofcir.io/ofcir:latest .
	kubectl delete deployment ofcir-controller-manager || true
	kubectl apply -f $(DEPLOY_MANIFESTS_DIR)/ofcir-operator.yaml || true

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest
MOCKGEN ?= $(LOCALBIN)/mockgen

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.14.0
MOCKGEN_VERSION ?= v1.6.0

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: mockgen
mockgen: $(MOCKGEN) ## Download mockgen locally if necessary.
$(MOCKGEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install github.com/golang/mock/mockgen@$(MOCKGEN_VERSION)

.PHONY: runapi
runapi: manifests generate fmt vet ## Run a controller from your host.
