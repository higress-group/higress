SHELL := /bin/bash -o pipefail

export HIGRESS_BASE_VERSION ?= 2023-07-20T20-50-43

export HUB ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/higress

export ISTIO_BASE_REGISTRY ?= $(HUB)

export BASE_VERSION ?= $(HIGRESS_BASE_VERSION)

export CHARTS ?= higress-registry.cn-hangzhou.cr.aliyuncs.com/charts

VERSION_PACKAGE := github.com/alibaba/higress/v2/pkg/cmd/lversion

GIT_COMMIT:=$(shell git rev-parse HEAD)

GO_LDFLAGS += -X $(VERSION_PACKAGE).higressVersion=$(shell cat VERSION) \
	-X $(VERSION_PACKAGE).gitCommitID=$(GIT_COMMIT)

GO ?= go

export GOPROXY ?= https://proxy.golang.org,direct

TARGET_ARCH ?= amd64

VALID_ARCHS := amd64 arm64
ifeq ($(filter $(TARGET_ARCH),$(VALID_ARCHS)),)
  $(error "TARGET_ARCH must be one of: $(VALID_ARCHS)")
endif

GOARCH_LOCAL := $(TARGET_ARCH)
GOOS_LOCAL := $(TARGET_OS)
RELEASE_LDFLAGS='$(GO_LDFLAGS) -extldflags -static -s -w'

export OUT:=$(TARGET_OUT)
export OUT_LINUX:=$(TARGET_OUT_LINUX)

BUILDX_PLATFORM ?=

# If tag not explicitly set in users' .istiorc.mk or command line, default to the git sha.
TAG ?= $(shell git rev-parse --verify HEAD)
ifeq ($(TAG),)
  $(error "TAG cannot be empty")
endif

VARIANT :=
ifeq ($(VARIANT),)
  TAG_VARIANT:=${TAG}
else
  TAG_VARIANT:=${TAG}-${VARIANT}
endif

HIGRESS_DOCKER_BUILD_TOP:=${OUT_LINUX}/docker_build

HIGRESS_BINARIES:=./cmd/higress

HGCTL_PROJECT_DIR=./hgctl
HGCTL_BINARIES:=./cmd/hgctl

$(OUT):
	@mkdir -p $@

submodule:
	git submodule update --init
#	git submodule update --remote

.PHONY: prebuild
prebuild: submodule
	./tools/hack/prebuild.sh

.PHONY: default
default: build

.PHONY: go.test.coverage
go.test.coverage: prebuild
	go test ./cmd/... ./pkg/... -race -coverprofile=coverage.xml -covermode=atomic

.PHONY: build
build: prebuild $(OUT)
	GOPROXY="$(GOPROXY)" GOOS=$(GOOS_LOCAL) GOARCH=$(GOARCH_LOCAL) LDFLAGS=$(RELEASE_LDFLAGS) tools/hack/gobuild.sh $(OUT)/ $(HIGRESS_BINARIES)

.PHONY: build-linux
build-linux: prebuild $(OUT)
	GOPROXY="$(GOPROXY)" GOOS=linux GOARCH=$(GOARCH_LOCAL) LDFLAGS=$(RELEASE_LDFLAGS) tools/hack/gobuild.sh $(OUT_LINUX)/ $(HIGRESS_BINARIES)

$(AMD64_OUT_LINUX)/higress:
	GOPROXY="$(GOPROXY)" GOOS=linux GOARCH=amd64 LDFLAGS=$(RELEASE_LDFLAGS) tools/hack/gobuild.sh ./out/linux_amd64/ $(HIGRESS_BINARIES)

$(ARM64_OUT_LINUX)/higress:
	GOPROXY="$(GOPROXY)" GOOS=linux GOARCH=arm64 LDFLAGS=$(RELEASE_LDFLAGS) tools/hack/gobuild.sh ./out/linux_arm64/ $(HIGRESS_BINARIES)

.PHONY: build-hgctl
build-hgctl: prebuild $(OUT)
	GOPROXY=$(GOPROXY) GOOS=$(GOOS_LOCAL) GOARCH=$(GOARCH_LOCAL) LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh $(OUT)/ $(HGCTL_BINARIES)

.PHONY: build-linux-hgctl
build-linux-hgctl: prebuild $(OUT)
	GOPROXY=$(GOPROXY) GOOS=linux GOARCH=$(GOARCH_LOCAL) LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh $(OUT_LINUX)/ $(HGCTL_BINARIES)

.PHONY: build-hgctl-multiarch
build-hgctl-multiarch: prebuild $(OUT)
	GOPROXY=$(GOPROXY) GOOS=linux GOARCH=amd64 LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/linux_amd64/ $(HGCTL_BINARIES)
	GOPROXY=$(GOPROXY) GOOS=linux GOARCH=arm64 LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/linux_arm64/ $(HGCTL_BINARIES)
	GOPROXY=$(GOPROXY) GOOS=windows GOARCH=amd64 LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/windows_amd64/ $(HGCTL_BINARIES)
	GOPROXY=$(GOPROXY) GOOS=windows GOARCH=arm64 LDFLAGS=$(RELEASE_LDFLAGS) PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/windows_arm64/ $(HGCTL_BINARIES)

.PHONY: build-hgctl-macos-arm64
build-hgctl-macos-arm64: prebuild $(OUT)
	CGO_ENABLED=1 STATIC=0 GOPROXY=$(GOPROXY) GOOS=darwin GOARCH=arm64 PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/darwin_arm64/ $(HGCTL_BINARIES)

.PHONY: build-hgctl-macos-amd64
build-hgctl-macos-amd64: prebuild $(OUT)
	CGO_ENABLED=1 STATIC=0 GOPROXY=$(GOPROXY) GOOS=darwin GOARCH=amd64 PROJECT_DIR="$(HGCTL_PROJECT_DIR)" tools/hack/gobuild.sh ../out/darwin_amd64/ $(HGCTL_BINARIES)

# Create targets for OUT_LINUX/binary
# There are two use cases here:
# * Building all docker images (generally in CI). In this case we want to build everything at once, so they share work
# * Building a single docker image (generally during dev). In this case we just want to build the single binary alone
BUILD_ALL ?= true
define build-linux
.PHONY: $(OUT_LINUX)/$(shell basename $(1))
ifeq ($(BUILD_ALL),true)
$(OUT_LINUX)/$(shell basename $(1)): build-linux
else
$(OUT_LINUX)/$(shell basename $(1)): $(OUT_LINUX)
	GOPROXY=$(GOPROXY) GOOS=linux GOARCH=$(GOARCH_LOCAL) LDFLAGS=$(RELEASE_LDFLAGS) tools/hack/gobuild.sh $(OUT_LINUX)/ -tags=$(2) $(1)
endif
endef

$(foreach bin,$(HIGRESS_BINARIES),$(eval $(call build-linux,$(bin),"")))

# Create helper targets for each binary, like "pilot-discovery"
# As an optimization, these still build everything
$(foreach bin,$(HIGRESS_BINARIES),$(shell basename $(bin))): build
ifneq ($(OUT_LINUX),$(LOCAL_OUT))
# if we are on linux already, then this rule is handled by build-linux above, which handles BUILD_ALL variable
$(foreach bin,$(HIGRESS_BINARIES),${LOCAL_OUT}/$(shell basename $(bin))): build
endif

.PHONY: push

# for now docker is limited to Linux compiles - why ?
include docker/docker.mk

docker-build-amd64: clean-higress docker.higress-amd64 ## Build and push amdd64 docker images to registry defined by $HUB and $TAG

docker-build: clean-higress docker.higress ## Build and push docker images to registry defined by $HUB and $TAG

docker-buildx-push: clean-env docker.higress-buildx

export PARENT_GIT_TAG:=$(shell cat VERSION)
export PARENT_GIT_REVISION:=$(TAG)

export ENVOY_PACKAGE_URL_PATTERN?=https://github.com/higress-group/proxy/releases/download/v2.2.1/envoy-symbol-ARCH.tar.gz

build-envoy: prebuild
	./tools/hack/build-envoy.sh

build-pilot: prebuild
	TARGET_ARCH=amd64 ./tools/hack/build-istio-pilot.sh
	TARGET_ARCH=arm64 ./tools/hack/build-istio-pilot.sh

build-pilot-local: prebuild
	TARGET_ARCH=${TARGET_ARCH} ./tools/hack/build-istio-pilot.sh

buildx-prepare:
	docker buildx inspect multi-arch >/dev/null 2>&1 || docker buildx create --name multi-arch --platform linux/amd64,linux/arm64 --use

build-gateway: prebuild buildx-prepare build-golang-filter
	USE_REAL_USER=1 TARGET_ARCH=amd64 DOCKER_TARGETS="docker.proxyv2" ./tools/hack/build-istio-image.sh init
	USE_REAL_USER=1 TARGET_ARCH=arm64 DOCKER_TARGETS="docker.proxyv2" ./tools/hack/build-istio-image.sh init
	DOCKER_TARGETS="docker.proxyv2" IMG_URL="${IMG_URL}" ./tools/hack/build-istio-image.sh docker.buildx

build-gateway-local: prebuild $(if $(filter amd64,$(TARGET_ARCH)),build-golang-filter-amd64,build-golang-filter-arm64)
	TARGET_ARCH=${TARGET_ARCH} DOCKER_TARGETS="docker.proxyv2" ./tools/hack/build-istio-image.sh docker

build-golang-filter-amd64:
	TARGET_ARCH=amd64 ./tools/hack/build-golang-filters.sh

build-golang-filter-arm64:
	TARGET_ARCH=arm64 ./tools/hack/build-golang-filters.sh

build-golang-filter:
	TARGET_ARCH=amd64 ./tools/hack/build-golang-filters.sh
	TARGET_ARCH=arm64 ./tools/hack/build-golang-filters.sh

build-istio: prebuild buildx-prepare
	DOCKER_TARGETS="docker.pilot" IMG_URL="${IMG_URL}" ./tools/hack/build-istio-image.sh docker.buildx

build-istio-local: prebuild
	TARGET_ARCH=${TARGET_ARCH} DOCKER_TARGETS="docker.pilot" ./tools/hack/build-istio-image.sh docker

build-wasmplugins:
	./tools/hack/build-wasm-plugins.sh

pre-install:
	cp api/kubernetes/customresourcedefinitions.gen.yaml helm/core/crds

define create_ns
   kubectl get namespace | grep $(1) || kubectl create namespace $(1)
endef

install: pre-install
	cd helm/higress; helm dependency build
	helm install higress helm/higress -n higress-system --create-namespace --set 'global.local=true'

HIGRESS_LATEST_IMAGE_TAG ?= latest
ENVOY_LATEST_IMAGE_TAG ?= 36c1d07376bf11295edc40357d74a5ecb50122b1
ISTIO_LATEST_IMAGE_TAG ?= 36c1d07376bf11295edc40357d74a5ecb50122b1

install-dev: pre-install
	helm install higress helm/core -n higress-system --create-namespace --set 'controller.tag=$(TAG)' --set 'gateway.replicas=1' --set 'pilot.tag=$(ISTIO_LATEST_IMAGE_TAG)' --set 'gateway.tag=$(ENVOY_LATEST_IMAGE_TAG)' --set 'global.local=true'
install-dev-wasmplugin: build-wasmplugins pre-install
	helm install higress helm/core -n higress-system --create-namespace --set 'controller.tag=$(TAG)' --set 'gateway.replicas=1' --set 'pilot.tag=$(ISTIO_LATEST_IMAGE_TAG)' --set 'gateway.tag=$(ENVOY_LATEST_IMAGE_TAG)' --set 'global.local=true'  --set 'global.volumeWasmPlugins=true' --set 'global.onlyPushRouteCluster=false'

uninstall:
	helm uninstall higress -n higress-system

upgrade: pre-install
	cd helm/higress; helm dependency build
	helm upgrade higress helm/higress -n higress-system --set 'global.local=true'

helm-push:
	cp api/kubernetes/customresourcedefinitions.gen.yaml helm/core/crds
	cd helm; tar -zcf higress.tgz higress; helm push higress.tgz "oci://$(CHARTS)"

cue = cue-gen -paths=./external/api/common-protos

gen-api: prebuild
	cd api;./gen.sh

gen-client: gen-api
	cd client; make generate-k8s-client

DIRS_TO_CLEAN := $(OUT)
DIRS_TO_CLEAN += $(OUT_LINUX)

clean-higress: ## Cleans all the intermediate files and folders previously generated.
	rm -rf $(DIRS_TO_CLEAN)

clean-istio:
	rm -rf external/api
	rm -rf external/client-go
	rm -rf external/istio
	rm -rf external/pkg

clean-gateway: clean-istio
	rm -rf external/envoy
	rm -rf external/proxy
	rm -rf external/go-control-plane
	rm -rf external/package/envoy.tar.gz
	rm -rf external/package/*.so

clean-env:
	rm -rf out/

clean-tool:
	rm -rf tools/bin

clean: clean-higress clean-gateway clean-istio clean-env clean-tool

include tools/tools.mk
include tools/lint.mk

# gateway-conformance-test runs gateway api conformance tests.
.PHONY: gateway-conformance-test
gateway-conformance-test:

# higress-conformance-test-prepare prepares the environment for higress conformance tests.
.PHONY: higress-conformance-test-prepare
higress-conformance-test-prepare: $(tools/kind) delete-cluster create-cluster docker-build kube-load-image install-dev

# higress-conformance-test runs ingress api conformance tests.
.PHONY: higress-conformance-test
higress-conformance-test: $(tools/kind) delete-cluster create-cluster docker-build kube-load-image install-dev run-higress-e2e-test delete-cluster

# higress-conformance-test-clean cleans the environment for higress conformance tests.
.PHONY: higress-conformance-test-clean
higress-conformance-test-clean: $(tools/kind) delete-cluster

# higress-wasmplugin-test-prepare prepares the environment for higress wasmplugin tests.
.PHONY: higress-wasmplugin-test-prepare
higress-wasmplugin-test-prepare: $(tools/kind) delete-cluster create-cluster docker-build kube-load-image install-dev-wasmplugin

# higress-wasmplugin-test-prepare-skip-docker-build prepares the environment for higress wasmplugin tests without build higress docker image.
.PHONY: higress-wasmplugin-test-prepare-skip-docker-build
higress-wasmplugin-test-prepare-skip-docker-build: $(tools/kind) delete-cluster create-cluster prebuild
	@export TAG="$(HIGRESS_LATEST_IMAGE_TAG)" && \
	$(MAKE) kube-load-image && \
	$(MAKE) install-dev-wasmplugin

# higress-wasmplugin-test runs ingress wasmplugin tests.
.PHONY: higress-wasmplugin-test
higress-wasmplugin-test: $(tools/kind) delete-cluster create-cluster docker-build kube-load-image install-dev-wasmplugin run-higress-e2e-test-wasmplugin delete-cluster

# higress-wasmplugin-test-skip-docker-build runs ingress wasmplugin tests without build higress docker image
.PHONY: higress-wasmplugin-test-skip-docker-build
higress-wasmplugin-test-skip-docker-build: $(tools/kind) delete-cluster create-cluster prebuild
	@export TAG="$(HIGRESS_LATEST_IMAGE_TAG)" && \
	$(MAKE) kube-load-image && \
	$(MAKE) install-dev-wasmplugin && \
	$(MAKE) run-higress-e2e-test-wasmplugin && \
	$(MAKE) delete-cluster

# higress-wasmplugin-test-clean cleans the environment for higress wasmplugin tests.
.PHONY: higress-wasmplugin-test-clean
higress-wasmplugin-test-clean: $(tools/kind) delete-cluster

# create-cluster creates a kube cluster with kind.
.PHONY: create-cluster
create-cluster: $(tools/kind)
	tools/hack/create-cluster.sh

# delete-cluster deletes a kube cluster.
.PHONY: delete-cluster
delete-cluster: $(tools/kind) ## Delete kind cluster.
	$(tools/kind) delete cluster --name higress

# kube-load-image loads a local built docker image into kube cluster.
# dubbo-provider-demo和nacos-standlone-rc3的镜像已经上传到阿里云镜像库，第一次需要先拉到本地
# docker pull registry.cn-hangzhou.aliyuncs.com/hinsteny/dubbo-provider-demo:0.0.1
# docker pull registry.cn-hangzhou.aliyuncs.com/hinsteny/nacos-standlone-rc3:1.0.0-RC3
# If TAG is HIGRESS_LATEST_IMAGE_TAG, means we skip building higress docker image, so we need to pull the image first.
.PHONY: kube-load-image
kube-load-image: $(tools/kind) ## Install the Higress image to a kind cluster using the provided $IMAGE and $TAG.
	@if [ "$(TAG)" = "$(HIGRESS_LATEST_IMAGE_TAG)" ]; then \
		tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/higress $(TAG); \
	fi
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/higress $(TAG)
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot $(ISTIO_LATEST_IMAGE_TAG)
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway $(ENVOY_LATEST_IMAGE_TAG)
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/dubbo-provider-demo 0.0.3-x86
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/nacos-standlone-rc3 1.0.0-RC3
	tools/hack/docker-pull-image.sh docker.io/hashicorp/consul 1.16.0
	tools/hack/docker-pull-image.sh docker.io/charlie1380/eureka-registry-provider v0.3.0
	tools/hack/docker-pull-image.sh docker.io/bitinit/eureka latest
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/httpbin 1.0.2
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-server 1.3.0
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-server v1.0
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-body 1.0.0
	tools/hack/docker-pull-image.sh openpolicyagent/opa 0.61.0
	tools/hack/docker-pull-image.sh curlimages/curl latest
	tools/hack/docker-pull-image.sh registry.cn-hangzhou.aliyuncs.com/2456868764/httpbin 1.0.2
	tools/hack/docker-pull-image.sh registry.cn-hangzhou.aliyuncs.com/hinsteny/nacos-standlone-rc3 1.0.0-RC3
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/dubbo-provider-demo 0.0.3-x86
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/nacos-standlone-rc3 1.0.0-RC3
	tools/hack/kind-load-image.sh docker.io/hashicorp/consul 1.16.0
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/httpbin 1.0.2
	tools/hack/kind-load-image.sh docker.io/charlie1380/eureka-registry-provider v0.3.0
	tools/hack/kind-load-image.sh docker.io/bitinit/eureka latest
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-server 1.3.0
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-server v1.0
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/echo-body 1.0.0
	tools/hack/kind-load-image.sh openpolicyagent/opa 0.61.0
	tools/hack/kind-load-image.sh curlimages/curl latest
	tools/hack/kind-load-image.sh registry.cn-hangzhou.aliyuncs.com/2456868764/httpbin 1.0.2
	tools/hack/kind-load-image.sh registry.cn-hangzhou.aliyuncs.com/hinsteny/nacos-standlone-rc3 1.0.0-RC3

# run-higress-e2e-test-setup starts to setup ingress e2e tests.
.PHONT: run-higress-e2e-test-setup
run-higress-e2e-test-setup:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go --ingress-class=higress --debug=true --test-area=setup

# run-higress-e2e-test starts to run ingress e2e tests.
.PHONY: run-higress-e2e-test
run-higress-e2e-test:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go --ingress-class=higress --debug=true --test-area=all --execute-tests=$(TEST_SHORTNAME)

# run-higress-e2e-test-run starts to run ingress e2e conformance tests.
.PHONY: run-higress-e2e-test-run
run-higress-e2e-test-run:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go --ingress-class=higress --debug=true --test-area=run --execute-tests=$(TEST_SHORTNAME)

# run-higress-e2e-test-clean starts to clean ingress e2e tests.
.PHONY: run-higress-e2e-test-clean
run-higress-e2e-test-clean:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go --ingress-class=higress --debug=true --test-area=clean

# run-higress-e2e-test-wasmplugin-setup starts to prepare ingress e2e tests.
.PHONY: run-higress-e2e-test-wasmplugin-setup
run-higress-e2e-test-wasmplugin-setup:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go -isWasmPluginTest=true -wasmPluginType=$(PLUGIN_TYPE) -wasmPluginName=$(PLUGIN_NAME) --ingress-class=higress --debug=true --test-area=setup

# run-higress-e2e-test-wasmplugin starts to run ingress e2e tests.
.PHONY: run-higress-e2e-test-wasmplugin
run-higress-e2e-test-wasmplugin:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go -isWasmPluginTest=true -wasmPluginType=$(PLUGIN_TYPE) -wasmPluginName=$(PLUGIN_NAME) --ingress-class=higress --debug=true --test-area=all --execute-tests=$(TEST_SHORTNAME)

# run-higress-e2e-test-wasmplugin-run starts to run ingress e2e conformance tests.
.PHONY: run-higress-e2e-test-wasmplugin-run
run-higress-e2e-test-wasmplugin-run:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go -isWasmPluginTest=true -wasmPluginType=$(PLUGIN_TYPE) -wasmPluginName=$(PLUGIN_NAME) --ingress-class=higress --debug=true --test-area=run --execute-tests=$(TEST_SHORTNAME)

# run-higress-e2e-test-wasmplugin-clean starts to clean ingress e2e tests.
.PHONY: run-higress-e2e-test-wasmplugin-clean
run-higress-e2e-test-wasmplugin-clean:
	@echo -e "\n\033[36mRunning higress conformance tests...\033[0m"
	@echo -e "\n\033[36mWaiting higress-controller to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-controller --for=condition=Available
	@echo -e "\n\033[36mWaiting higress-gateway to be ready...\033[0m\n"
	kubectl wait --timeout=10m -n higress-system deployment/higress-gateway --for=condition=Available
	go test -v -tags conformance ./test/e2e/e2e_test.go -isWasmPluginTest=true -wasmPluginType=$(PLUGIN_TYPE) -wasmPluginName=$(PLUGIN_NAME) --ingress-class=higress --debug=true --test-area=clean
