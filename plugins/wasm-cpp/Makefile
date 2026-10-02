PLUGIN_NAME ?= key_auth
BUILD_TIME := $(shell date "+%Y%m%d-%H%M%S")
COMMIT_ID := $(shell git rev-parse --short HEAD 2>/dev/null)
VERSION_FILE := $(wildcard extensions/${PLUGIN_NAME}/VERSION)
FILE_VERSION := $(if ${VERSION_FILE},$(strip $(shell cat ${VERSION_FILE} 2>/dev/null | tr -d '[:space:]')))
IMAGE_TAG = $(if $(strip $(PLUGIN_VERSION)),${PLUGIN_VERSION},$(if ${FILE_VERSION},${FILE_VERSION},${BUILD_TIME}-${COMMIT_ID}))
IMG ?= ${REGISTRY}${PLUGIN_NAME}:${IMAGE_TAG}

.PHONY: build
build:
	DOCKER_BUILDKIT=1 docker build --build-arg PLUGIN_NAME=${PLUGIN_NAME} \
	                            -t ${IMG} \
	                            --output extensions/${PLUGIN_NAME} \
	                            .
	@echo ""
	@echo "output wasm file: extensions/${PLUGIN_NAME}/plugin.wasm"
