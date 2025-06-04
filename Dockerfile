# Build the manager binary using CentOS Stream 9 as the base image
FROM quay.io/centos/centos:stream9 as builder

WORKDIR /workspace

# Install necessary tools and dependencies in a single RUN command to minimize image layers
RUN dnf -y install epel-release && \
    dnf config-manager --set-enabled crb && \
    dnf -y install gcc glibc-devel glibc-headers libvirt-devel go && \
    dnf clean all

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY cmd/ cmd/
COPY controllers/ controllers/
COPY pkg/ pkg/

# Build the operator
RUN CGO_ENABLED=1 go build -a -o ofcir-operator main.go

# Build the api server
RUN CGO_ENABLED=0 go build -a -o ofcir-api cmd/ofcir-api/main.go

# Cleanup
RUN dnf remove -y gcc glibc-devel glibc-headers libvirt-devel go && \
    dnf clean all && \
    rm -rf /var/cache/yum

# Final stage - create the runtime image
FROM quay.io/centos/centos:stream9 as runtime

WORKDIR /

RUN dnf -y install libvirt-libs && \
    dnf clean all

# Copy the binaries from the builder stage
COPY --from=builder /workspace/ofcir-operator .
COPY --from=builder /workspace/ofcir-api .

ENTRYPOINT ["/ofcir-api"]
