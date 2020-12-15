# Build the manager binary
FROM golang:1.14.4 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY . /workspace/src/github.com/AliyunContainerService/et-operator

# Build
RUN cd /workspace/src/github.com/AliyunContainerService/et-operator/hack/kubexec && CGO_ENABLED=0 GOPATH=/workspace GOOS=linux GOARCH=amd64 GO111MODULE=off go build -a -o /bin/kubexec main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM alpine:3.11.6
WORKDIR /
COPY --from=builder /bin/kubexec .

ENTRYPOINT ["/bin/sh", "-c"]
CMD ["cp /bin/kubexec /opt/kube/kubexec.sh"]