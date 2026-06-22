#!/usr/bin/env bash
# Local e2e helper. OpenShift CI does not use this script; see hack/ci-e2e-test.sh.
#
# CI (release repo e2e-tests job): remote host runs "make e2e-tests" after installing
# go, make, podman, and kubectl (same as hack/ci-e2e-test.sh).
#
# Local workflows:
#   make e2e-ci / make e2e-tests     Emulate CI (only the go test step)
#   make local-e2e / hack/local-e2e.sh       Fast checks + emulate CI
#   hack/local-e2e.sh --skip-checks          Same as make e2e-ci, with optional -r/--timeout
#   hack/local-e2e.sh --setup-only           Debug kind cluster + deploy (not used in CI)
#
# Environment (passed through to make e2e-tests):
#   E2E_TIMEOUT, E2E_RUN, IMG, OFCIR_IMAGE, KIND_CLUSTER, KIND_EXPERIMENTAL_PROVIDER

set -euo pipefail

[[ "${DEBUG:-}" == "true" ]] && set -x

readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

readonly DEFAULT_IMG="localhost/ofcir-test:latest"
readonly DEFAULT_KIND_CLUSTER="ofcir-test"
readonly DEFAULT_E2E_TIMEOUT="20m"
readonly IMAGE_ARCHIVE="/tmp/ofcir-latest.tar"
readonly E2E_DIR="${REPO_ROOT}/tests/e2e"

IMG="${IMG:-${DEFAULT_IMG}}"
KIND_CLUSTER="${KIND_CLUSTER:-${DEFAULT_KIND_CLUSTER}}"
E2E_TIMEOUT="${E2E_TIMEOUT:-${DEFAULT_E2E_TIMEOUT}}"
export E2E_TIMEOUT

RUN_CHECKS=true
RUN_E2E=true
SETUP_ONLY=false
TEARDOWN_ONLY=false
E2E_RUN=""
SKIP_BUILD=false

log() {
	printf '%s\n' "$*"
}

die() {
	printf 'error: %s\n' "$*" >&2
	exit 1
}

usage() {
	cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Local helper around "make e2e-tests". CI runs that make target on a remote host
via hack/ci-e2e-test.sh (see openshift/release ci-operator config).

Modes:
  (default)            Fast checks (kustomize, unit) then make e2e-tests
  --skip-checks        Emulate CI: only make e2e-tests (same as make e2e-ci)

Options:
  -h, --help           Show this help
  --checks-only        Fast checks only (no cluster / no e2e)
  --setup-only         kind + image + deploy (debug; not part of CI)
  --teardown           Delete kind cluster ${DEFAULT_KIND_CLUSTER}
  --skip-build         Skip image build in TestMain (sets OFCIR_IMAGE=\$IMG)
  -r, --run PATTERN    Passed to go test via E2E_RUN
  --img IMAGE          Operator image (default: ${DEFAULT_IMG})
  --timeout DURATION   go test timeout (default: ${DEFAULT_E2E_TIMEOUT})

Examples:
  make e2e-ci
  make local-e2e
  $(basename "$0") --skip-checks
  $(basename "$0") -r '^TestAcquire$'
  $(basename "$0") --checks-only
EOF
}

parse_args() {
	while [[ $# -gt 0 ]]; do
		case "$1" in
		-h | --help)
			usage
			exit 0
			;;
		--checks-only)
			RUN_E2E=false
			SETUP_ONLY=false
			shift
			;;
		--setup-only)
			RUN_E2E=false
			RUN_CHECKS=false
			SETUP_ONLY=true
			shift
			;;
		--teardown)
			TEARDOWN_ONLY=true
			shift
			;;
		--skip-checks)
			RUN_CHECKS=false
			shift
			;;
		--skip-build)
			SKIP_BUILD=true
			shift
			;;
		-r | --run)
			[[ $# -lt 2 ]] && die "missing value for $1"
			E2E_RUN="$2"
			export E2E_RUN
			shift 2
			;;
		--img)
			[[ $# -lt 2 ]] && die "missing value for $1"
			IMG="$2"
			shift 2
			;;
		--timeout)
			[[ $# -lt 2 ]] && die "missing value for $1"
			E2E_TIMEOUT="$2"
			export E2E_TIMEOUT
			shift 2
			;;
		--ci | --local)
			die "$1 is no longer used; use --skip-checks or make e2e-ci to emulate CI"
			;;
		*)
			die "unknown argument: $1 (use --help)"
			;;
		esac
	done
}

require_command() {
	local cmd="$1"
	local hint="${2:-}"

	if command -v "${cmd}" >/dev/null 2>&1; then
		return 0
	fi
	if [[ -n "${hint}" ]]; then
		die "required command not found: ${cmd} (${hint})"
	fi
	die "required command not found: ${cmd}"
}

configure_kind_provider() {
	if [[ -n "${KIND_EXPERIMENTAL_PROVIDER:-}" ]]; then
		log "Using KIND_EXPERIMENTAL_PROVIDER=${KIND_EXPERIMENTAL_PROVIDER}"
		return 0
	fi

	if command -v podman >/dev/null 2>&1 && ! docker info >/dev/null 2>&1; then
		export KIND_EXPERIMENTAL_PROVIDER=podman
		log "Detected podman without docker; set KIND_EXPERIMENTAL_PROVIDER=podman"
	fi
}

require_container_runtime() {
	if command -v podman >/dev/null 2>&1; then
		return 0
	fi
	if docker info >/dev/null 2>&1; then
		log "podman not found; using docker"
		return 0
	fi
	die "need podman or docker (installed on the CI test host via ci-e2e-test.sh)"
}

warn_go_version() {
	local go_version
	go_version="$(grep -E '^go [0-9]' "${REPO_ROOT}/go.mod" | awk '{print $2}')"
	if [[ -z "${go_version}" ]]; then
		return 0
	fi
	local installed_go
	installed_go="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
	if [[ -n "${installed_go}" && "${installed_go}" != "${go_version}" ]]; then
		log "warning: go.mod wants go ${go_version}, installed go ${installed_go}"
	fi
}

# Same tools hack/ci-e2e-test.sh installs on the remote test host before make e2e-tests.
check_ci_prerequisites() {
	log "Checking prerequisites (emulating CI remote host)..."
	require_command go
	require_command make
	require_command kubectl
	require_container_runtime
	configure_kind_provider
	warn_go_version
	log "Prerequisites OK"
}

check_local_prerequisites() {
	log "Checking prerequisites (local full run)..."
	check_ci_prerequisites
	require_command kind
	log "Prerequisites OK"
}

run_fast_checks() {
	log "Running fast checks (kustomize + unit tests)..."
	cd "${REPO_ROOT}"
	make kustomize
	if [[ -f "${REPO_ROOT}/config/manifest_test.go" ]]; then
		go test ./config/ -count=1
	fi
	make unit-tests
	log "Fast checks passed"
}

restore_manager_kustomization() {
	if git -C "${REPO_ROOT}" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		git -C "${REPO_ROOT}" checkout -- config/manager/kustomization.yaml 2>/dev/null || true
	fi
}

teardown_cluster() {
	require_command kind
	log "Deleting kind cluster ${KIND_CLUSTER}..."
	if kind delete cluster --name "${KIND_CLUSTER}"; then
		log "Cluster deleted"
	else
		log "Cluster ${KIND_CLUSTER} was not present or could not be deleted"
	fi
}

setup_cluster() {
	require_command kind
	local kind_config="${E2E_DIR}/kind-config.yaml"
	[[ -f "${kind_config}" ]] || die "missing ${kind_config}"

	log "Creating kind cluster ${KIND_CLUSTER}..."
	if kind get clusters 2>/dev/null | grep -qx "${KIND_CLUSTER}"; then
		log "Cluster ${KIND_CLUSTER} already exists; reusing it"
	else
		kind create cluster --name "${KIND_CLUSTER}" --config "${kind_config}"
	fi

	local kubeconfig
	kubeconfig="$(kind get kubeconfig --name "${KIND_CLUSTER}")"
	export KUBECONFIG="${kubeconfig}"

	if [[ -f "${kubeconfig}" ]]; then
		sed -i '/enabling experimental podman provider/d' "${kubeconfig}"
	fi

	cd "${REPO_ROOT}"
	make kustomize

	if [[ "${SKIP_BUILD}" == "true" ]]; then
		log "Skipping image build (--skip-build)"
	else
		log "Building and loading image ${IMG}..."
		make IMG="${IMG}" ofcir-image
		rm -f "${IMAGE_ARCHIVE}"
		podman save -o "${IMAGE_ARCHIVE}" "${IMG}"
		kind load image-archive --name "${KIND_CLUSTER}" "${IMAGE_ARCHIVE}"
	fi

	log "Deploying operator (KUSTOMIZE_BUILD_DIR=config/e2e)..."
	make IMG="${IMG}" KUSTOMIZE_BUILD_DIR=config/e2e deploy
	restore_manager_kustomization

	log "Waiting for deployment to become ready..."
	kubectl -n ofcir-system wait --for=condition=available deployment/ofcir-controller-manager --timeout=180s

	log "Setup complete."
	log "  KUBECONFIG=${kubeconfig}"
	log "  kubectl -n ofcir-system get pods"
	log "  API NodePort is mapped to host port 30007 (see tests/e2e/kind-config.yaml)"
}

run_e2e_tests() {
	cd "${REPO_ROOT}"

	if [[ "${SKIP_BUILD}" == "true" ]]; then
		export OFCIR_IMAGE="${IMG}"
		log "Skipping image build in TestMain (OFCIR_IMAGE=${OFCIR_IMAGE})"
	fi

	if [[ -n "${E2E_RUN}" ]]; then
		log "Running make e2e-tests (run=${E2E_RUN}, timeout=${E2E_TIMEOUT})..."
	else
		log "Running make e2e-tests (timeout=${E2E_TIMEOUT})..."
	fi

	make e2e-tests
}

main() {
	parse_args "$@"

	if [[ "${TEARDOWN_ONLY}" == "true" ]]; then
		teardown_cluster
		exit 0
	fi

	if [[ "${SETUP_ONLY}" == "true" ]]; then
		check_local_prerequisites
		setup_cluster
		exit 0
	fi

	if [[ "${RUN_CHECKS}" == "true" && "${RUN_E2E}" == "true" ]]; then
		check_local_prerequisites
	elif [[ "${RUN_CHECKS}" == "true" ]]; then
		check_ci_prerequisites
	elif [[ "${RUN_E2E}" == "true" ]]; then
		check_ci_prerequisites
	else
		check_ci_prerequisites
	fi

	if [[ "${RUN_CHECKS}" == "true" ]]; then
		run_fast_checks
	fi

	if [[ "${RUN_E2E}" == "true" ]]; then
		run_e2e_tests
	fi
}

main "$@"
