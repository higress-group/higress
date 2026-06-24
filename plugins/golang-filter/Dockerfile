FROM debian:bullseye AS golang-base

ARG GOPROXY
ARG GO_FILTER_NAME
ARG GOARCH=amd64
ARG GO_VERSION=1.25.11
ARG GO_AMD64_SHA256=34f14304e856893f4ba30c2cacfe93906e9de7915c5f6aaaf3a81cdccd7ba30b
ARG GO_ARM64_SHA256=c30bf9e156a54ea4e31fbbbf31a712b32734b58cc9a22426fa5ee632d0885124

ENV PATH=/usr/local/go/bin:$PATH
ENV GOFLAGS=-buildvcs=false
ENV GOPROXY=${GOPROXY}
ENV GOARCH=${GOARCH}
ENV CGO_ENABLED=1

# 根据目标架构安装对应的编译工具
RUN set -eux; \
    apt-get update; \
    apt-get install --no-install-recommends -y ca-certificates curl git tar; \
    if [ "$GOARCH" = "arm64" ]; then \
        echo "Installing ARM64 toolchain" && \
        apt-get install --no-install-recommends -y gcc-aarch64-linux-gnu binutils-aarch64-linux-gnu libc6-dev-arm64-cross && \
        GO_SHA256="$GO_ARM64_SHA256"; \
    else \
        echo "Installing AMD64 toolchain" && \
        apt-get install --no-install-recommends -y gcc-x86-64-linux-gnu binutils-x86-64-linux-gnu libc6-dev && \
        GO_SHA256="$GO_AMD64_SHA256"; \
    fi; \
    GO_TARBALL="go${GO_VERSION}.linux-${GOARCH}.tar.gz"; \
    curl -fsSLo "/tmp/${GO_TARBALL}" "https://go.dev/dl/${GO_TARBALL}"; \
    echo "${GO_SHA256}  /tmp/${GO_TARBALL}" | sha256sum -c -; \
    tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"; \
    rm -f "/tmp/${GO_TARBALL}"; \
    apt-get clean; \
    rm -rf /var/lib/apt/lists/*; \
    go version

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
