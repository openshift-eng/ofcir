# ofcir
A resource pooling service for metal CI jobs

## Description
Ofcir provides a unified and simplified way to seamlessly acquire and release resources from the available pools,
so that it will be possible to have a more efficient resource management and a more stable CI environment.

## Getting Started

Since we'll need a Kubernetes cluster to run against the operator, [Minikube](https://minikube.sigs.k8s.io/) could be used to quickly setup one. The following command could be used to install it
on a Fedora laptop:

```
curl -LO https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
sudo install minikube-linux-amd64 /usr/local/bin/minikube
```

Once done, you can start the cluster:

```
minikube start
```

### Running on the cluster
1. Install Instances of Custom Resources:

```sh
make install
```

2. Build and push your image to the location specified by `IMG`:
	
```sh
make ofcir-image IMG=<some-registry>/ofcir:tag
```
	
3. Deploy the controller to the cluster with the image specified by `IMG`:

```sh
make deploy IMG=<some-registry>/ofcir:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:

```sh
make uninstall
```

### Undeploy controller
UnDeploy the controller to the cluster:

```sh
make undeploy
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources untile the desired state is reached on the cluster

### Test It Out
1. Install the CRDs into the cluster:

```sh
make install
```

2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):

```sh
make run
```

**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:

```sh
make manifests
```

**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## End-to-end tests

E2e tests run in `tests/e2e` using [e2e-framework](https://github.com/kubernetes-sigs/e2e-framework).
`TestMain` creates a [kind](https://kind.sigs.k8s.io/) cluster, builds and loads the operator image with podman, deploys via kustomize, then runs the test package.
That flow is the same in CI and on your laptop; only the wrapper around `make e2e-tests` differs.

### How CI runs e2e

OpenShift CI is configured in [openshift/release](https://github.com/openshift/release) (`ci-operator/config/openshift-eng/ofcir/openshift-eng-ofcir-main.yaml`):

1. **pre** — `ofcir-acquire` provisions an Equinix metal host and writes `server-ip` to `SHARED_DIR`.
2. **test** — the `ofcir-tests-base` image runs `./ofcir/hack/ci-e2e-test.sh`.
3. **post** — `ofcir-release` tears down the CIR.

`hack/ci-e2e-test.sh` copies the repo to the remote host, installs Go (from `go.mod`), `make`, `podman`, and `kubectl`, then runs:

```sh
make e2e-tests
```

CI does not invoke `hack/local-e2e.sh`.
Keep `hack/ci-e2e-test.sh` and that make target as the CI entrypoint when changing local tooling.

### Local workflows

| Goal | Command |
|------|---------|
| Same as CI (only `go test`) | `make e2e-ci` or `make e2e-tests` |
| CI step + prereq check | `./hack/local-e2e.sh --skip-checks` |
| Recommended before push (unit/kustomize + e2e) | `make local-e2e` or `make e2e` |
| Unit/kustomize only | `./hack/local-e2e.sh --checks-only` |

`make e2e-ci` is an alias for `make e2e-tests` (the target CI runs on the remote host).
`make local-e2e` calls `hack/local-e2e.sh`, which runs fast checks then `make e2e-tests`.

### Prerequisites

For **CI emulation** (`make e2e-ci`), install what `hack/ci-e2e-test.sh` puts on the remote host, plus `kind` on your `PATH` (TestMain shells out to it):

- Go (version in `go.mod`)
- `make`, `kubectl`, `podman` (or Docker)
- `kind`

On Fedora with podman only, `hack/local-e2e.sh` sets `KIND_EXPERIMENTAL_PROVIDER=podman` when needed.

For **`make local-e2e`** (full local run), the same tools apply; `hack/local-e2e.sh` checks them before running tests.

### Examples

Emulate CI (single test):

```sh
E2E_RUN='^TestAcquire$' make e2e-ci
```

Longer timeout:

```sh
E2E_TIMEOUT=30m make e2e-ci
```

Full local run with fast checks:

```sh
make local-e2e
```

Optional `hack/local-e2e.sh` flags (local only): `-r` / `--run`, `--timeout`, `--skip-build`, `--setup-only`, `--teardown`.
Run `./hack/local-e2e.sh --help` for details.

## License

Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
