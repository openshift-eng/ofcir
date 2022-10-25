# Build the manager binary
FROM golang:1.17-alpine3.16 as builder

WORKDIR /workspace
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
RUN apk add libc-dev gcc
RUN apk add libvirt-dev
RUN CGO_ENABLED=1 go build -a -o ofcir-operator main.go

# Build the api server
RUN CGO_ENABLED=0 go build -a -o ofcir-api cmd/ofcir-api/main.go

# Cleanup 
FROM alpine:3.16

RUN apk add libc-dev gcc
RUN apk add libvirt-dev

WORKDIR /
COPY --from=builder /workspace/ofcir-operator .
COPY --from=builder /workspace/ofcir-api .

ENTRYPOINT ["/ofcir-api"]