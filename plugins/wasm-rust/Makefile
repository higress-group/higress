PLUGIN_NAME ?= ai-data-masking
PLUGIN_ROOT ?= extensions
BUILDER_REGISTRY ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/
REGISTRY ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/
RUST_VERSION ?= 1.82
ORAS_VERSION ?= 1.0.0
HIGRESS_VERSION ?= 1.0.0-rc
BUILDER ?= ${BUILDER_REGISTRY}wasm-rust-builder:rust${RUST_VERSION}-oras${ORAS_VERSION}
BUILD_TIME := $(shell date "+%Y%m%d-%H%M%S")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null)
IMAGE_TAG = $(if $(strip $(PLUGIN_VERSION)),${PLUGIN_VERSION},${BUILD_TIME}-${COMMIT_ID})
IMG ?= ${REGISTRY}${PLUGIN_NAME}:${IMAGE_TAG}

.DEFAULT:
build:
	DOCKER_BUILDKIT=1 docker build \
		--build-arg PLUGIN_NAME=${PLUGIN_NAME} \
		--build-arg PLUGIN_ROOT=${PLUGIN_ROOT} \
		-t ${IMG} \
	    --output ${PLUGIN_ROOT}/${PLUGIN_NAME} \
	    .
	@echo ""
	@echo "output wasm file: ${PLUGIN_ROOT}/${PLUGIN_NAME}/plugin.wasm"

build-image:
	DOCKER_BUILDKIT=1 docker build \
	    --build-arg PLUGIN_NAME=${PLUGIN_NAME} \
	    --build-arg PLUGIN_ROOT=${PLUGIN_ROOT} \
	    --build-arg BUILDER=${BUILDER}  \
	    -t ${IMG} \
	    .
	@echo ""
	@echo "image:            ${IMG}"

lint-base:
	cargo fmt --all --check
	cargo clippy --workspace --all-features --all-targets
	cargo clean
lint:
	cargo fmt --all --check --manifest-path ${PLUGIN_ROOT}/${PLUGIN_NAME}/Cargo.toml
	cargo clippy --workspace --all-features --all-targets --manifest-path ${PLUGIN_ROOT}/${PLUGIN_NAME}/Cargo.toml
	cargo clean --manifest-path ${PLUGIN_ROOT}/${PLUGIN_NAME}/Cargo.toml

test-base:
	cargo test --lib
	cargo clean

test:
	cargo test --manifest-path ${PLUGIN_ROOT}/${PLUGIN_NAME}/Cargo.toml
	cargo clean --manifest-path ${PLUGIN_ROOT}/${PLUGIN_NAME}/Cargo.toml

builder:
	DOCKER_BUILDKIT=1 docker build \
			--build-arg RUST_VERSION=$(RUST_VERSION) \
			--build-arg ORAS_VERSION=$(ORAS_VERSION) \
			--build-arg HIGRESS_VERSION=$(HIGRESS_VERSION) \
			-f DockerfileBuilder \
			-t ${BUILDER} \
			.
	@echo ""
	@echo "image: ${BUILDER}"
