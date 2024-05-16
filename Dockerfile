# Build the manager binary
FROM golang:1.22 AS builder

ARG TARGETOS
ARG TARGETARCH

RUN echo "deb http://archive.ubuntu.com/ubuntu/ focal main restricted universe multiverse" > /etc/apt/sources.list
RUN apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 3B4FE6ACC0B21F32 871920D1991BC93C

# 压缩镜像
RUN apt-get update -y --allow-unauthenticated && apt-get install -y upx


WORKDIR /workspace

COPY go.mod go.mod
COPY go.sum go.sum



COPY main.go main.go
COPY cmd/tls/main.go cmd/tls/main.go
COPY pkg/ pkg/


ENV CGO_ENABLED=0
ENV GOOS=linux
ENV GOARCH=amd64
ENV GO111MODULE=on
ENV GOPROXY=https://goproxy.cn,direct

# 进行压缩
RUN go mod download && \
    go build -a -o admission-registry main.go && \
    go build -a -o tls-manager cmd/tls/main.go && \
    upx admission-registry tls-manager


FROM alpine:3.12.0 as manager
COPY --from=builder /workspace/admission-registry .
ENTRYPOINT ["/admission-registry"]


FROM alpine:3.12.0 as tls
COPY --from=builder /workspace/tls-manager .
ENTRYPOINT ["/tls-manager"]