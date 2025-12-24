#!/bin/bash
# CI E2E Test Script
# This script is called by the OpenShift CI system to run e2e tests on a remote server.
#
# Required environment variables:
#   SHARED_DIR - Directory containing shared CI artifacts (server-ip file)
#   CLUSTER_PROFILE_DIR - Directory containing cluster profile (packet-ssh-key)
#
# Usage: ./hack/ci-e2e-test.sh

set -euo pipefail

# Validate required environment variables
if [[ -z "${SHARED_DIR:-}" ]]; then
    echo "ERROR: SHARED_DIR environment variable is not set"
    exit 1
fi

if [[ -z "${CLUSTER_PROFILE_DIR:-}" ]]; then
    echo "ERROR: CLUSTER_PROFILE_DIR environment variable is not set"
    exit 1
fi

# Read server IP from shared directory
SERVER_IP="$(cat "${SHARED_DIR}/server-ip")"
SSH_KEY="${CLUSTER_PROFILE_DIR}/packet-ssh-key"

# SSH options for connecting to the remote server
SSHOPTS=(
    -o 'ConnectTimeout=5'
    -o 'StrictHostKeyChecking=no'
    -o 'UserKnownHostsFile=/dev/null'
    -o 'ServerAliveInterval=90'
    -o 'LogLevel=ERROR'
    -i "$SSH_KEY"
)

echo "Setup test environment"
echo "Server IP: ${SERVER_IP}"

# Copy current ofcir sources to the remote server
tar -czf ofcir.tar.gz ofcir
scp "${SSHOPTS[@]}" ofcir.tar.gz "cloud-user@${SERVER_IP}:/tmp/ofcir.tar.gz"

# Run tests on remote server
timeout -s 9 60m ssh "${SSHOPTS[@]}" "cloud-user@${SERVER_IP}" bash - << 'EOF'
set -euo pipefail
sudo su

### Unpack ofcir sources
tar -xzvf /tmp/ofcir.tar.gz

### Install golang
curl -OL https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
export GOPATH=/usr/local/go
export PATH=$PATH:$GOPATH/bin

### Install dependencies
dnf install -y make podman
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl
export PATH=$PATH:/usr/local/bin

echo "Running e2e tests..."
cd ofcir
make e2e-tests
EOF

echo "E2E tests completed successfully"

