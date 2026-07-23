PLUGIN_NAME ?= hello-world
BUILDER_REGISTRY ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/
REGISTRY ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/plugins/
GO_VERSION ?= 1.24.4
TINYGO_VERSION ?= 0.29.0
ORAS_VERSION ?= 1.0.0
HIGRESS_VERSION ?= 1.0.0-rc
USE_HIGRESS_TINYGO ?= false
BUILDER ?= ${BUILDER_REGISTRY}wasm-go-builder:go${GO_VERSION}-oras${ORAS_VERSION}
BUILD_TIME := $(shell date "+%Y%m%d-%H%M%S")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null)
IMAGE_TAG = $(if $(strip $(PLUGIN_VERSION)),${PLUGIN_VERSION},${BUILD_TIME}-${COMMIT_ID})
IMG ?= ${REGISTRY}${PLUGIN_NAME}:${IMAGE_TAG}
GOPROXY := $(shell go env GOPROXY)

.DEFAULT:
build:
	DOCKER_BUILDKIT=1 docker build --build-arg PLUGIN_NAME=${PLUGIN_NAME} \
	                            --build-arg BUILDER=${BUILDER}  \
	                            --build-arg GOPROXY=$(GOPROXY) \
	                            -t ${IMG} \
	                            --output extensions/${PLUGIN_NAME} \
	                            .
	@echo ""
	@echo "output wasm file: extensions/${PLUGIN_NAME}/plugin.wasm"

build-image:
	DOCKER_BUILDKIT=1 docker build --build-arg PLUGIN_NAME=${PLUGIN_NAME} \
	                            --build-arg BUILDER=${BUILDER}  \
	                            --build-arg GOPROXY=$(GOPROXY) \
	                            -t ${IMG} \
	                            .
	@echo ""
	@echo "image:            ${IMG}"

build-push: build-image
	docker push ${IMG}

# builder:
# To build a wasm-go-builder image.
# e.g.
#   REGISTRY=<your_docker_registry> make builder
# If you want to use Go/TinyGo/Oras with another version, please modify GO_VERSION/TINYGO_VERSION/ORAS_VERSION.
# After your wasm-go-builder image is built, you can use it to build plugin image.
# e.g.
#   PLUGIN_NAME=request-block BUILDER=<your-wasm-go-builder> make
builder:
	docker buildx build --no-cache \
			--platform linux/amd64,linux/arm64 \
			--build-arg BASE_IMAGE=docker.io/ubuntu \
			--build-arg GO_VERSION=$(GO_VERSION) \
			--build-arg ORAS_VERSION=$(ORAS_VERSION) \
			--build-arg HIGRESS_VERSION=$(HIGRESS_VERSION) \
			--build-arg USE_HIGRESS_TINYGO=$(USE_HIGRESS_TINYGO) \
			-f DockerfileBuilder \
			-t ${BUILDER} \
			--push \
			.
	@echo ""
	@echo "image: ${BUILDER}"

local-build:
	cd extensions/${PLUGIN_NAME}; \
	if [ -f prepare.sh ]; then sh ./prepare.sh; fi; \
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o ./main.wasm .

	@echo ""
	@echo "wasm: extensions/${PLUGIN_NAME}/main.wasm"

# local-build-tinygo: Build with TinyGo toolchain (default, backward compatible)
local-build-tinygo:
	cd extensions/${PLUGIN_NAME}; \
	if [ -f prepare.sh ]; then sh ./prepare.sh; fi; \
	tinygo build -o ./main.wasm -scheduler=none -target=wasi .

	@echo ""
	@echo "wasm (TinyGo): extensions/${PLUGIN_NAME}/main.wasm"

# local-build-go-native: Build with Go 1.24+ native Wasm (wasip1)
local-build-go-native:
	cd extensions/${PLUGIN_NAME}; \
	if [ -f prepare.sh ]; then sh ./prepare.sh; fi; \
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -ldflags="-s -w" -o ./main.wasm .

	@echo ""
	@echo "wasm (Go Native): extensions/${PLUGIN_NAME}/main.wasm"

# local-build-auto: Auto-detect toolchain and build
# If TINYGO=true, use TinyGo; if GO_NATIVE=true, use Go native; otherwise auto-detect
local-build-auto:
	@if [ "$(TINYGO)" = "true" ]; then \
		echo "Building with TinyGo (TINYGO=true)..."; \
		$(MAKE) local-build-tinygo PLUGIN_NAME=${PLUGIN_NAME}; \
	elif [ "$(GO_NATIVE)" = "true" ]; then \
		echo "Building with Go native Wasm (GO_NATIVE=true)..."; \
		$(MAKE) local-build-go-native PLUGIN_NAME=${PLUGIN_NAME}; \
	elif command -v tinygo >/dev/null 2>&1 && [ -f extensions/${PLUGIN_NAME}/go.mod ]; then \
		echo "TinyGo detected, building with TinyGo..."; \
		$(MAKE) local-build-tinygo PLUGIN_NAME=${PLUGIN_NAME}; \
	else \
		echo "Building with Go native Wasm..."; \
		$(MAKE) local-build-go-native PLUGIN_NAME=${PLUGIN_NAME}; \
	fi

local-run:
	python3 .devcontainer/gen_config.py ${PLUGIN_NAME}
	envoy -c extensions/${PLUGIN_NAME}/config.yaml --concurrency 0 --log-level info --component-log-level wasm:debug

local-all: local-build local-run
