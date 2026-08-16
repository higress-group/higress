FROM golang:1.23-bullseye AS golang-base

ARG GOPROXY
ARG GO_FILTER_NAME
ARG GOARCH

ENV GOFLAGS=-buildvcs=false
ENV GOPROXY=${GOPROXY}
ENV GOARCH=${GOARCH}
ENV CGO_ENABLED=1

# 根据目标架构安装对应的编译工具
RUN if [ "$GOARCH" = "arm64" ]; then \
        echo "Installing ARM64 toolchain" && \
        apt-get update && \
        apt-get install -y gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu; \
    else \
        echo "Installing AMD64 toolchain" && \
        apt-get update && \
        apt-get install -y gcc-x86-64-linux-gnu binutils-x86-64-linux-gnu; \
    fi

WORKDIR /workspace

COPY . .

WORKDIR /workspace

RUN go mod tidy
RUN if [ "$GOARCH" = "arm64" ]; then \
        CC=aarch64-linux-gnu-gcc AS=aarch64-linux-gnu-as go build -o /$GO_FILTER_NAME.so -buildmode=c-shared .; \
    else \
        CC=x86_64-linux-gnu-gcc AS=x86_64-linux-gnu-as go build -o /$GO_FILTER_NAME.so -buildmode=c-shared .; \
    fi

FROM scratch AS output
ARG GO_FILTER_NAME
ARG GOARCH
COPY --from=golang-base /${GO_FILTER_NAME}.so golang-filter_${GOARCH}.so
