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

GATEWAY_API_VERSION ?= v1.6.0
GATEWAY_API_CRD_VERSIONS ?= $(GATEWAY_API_VERSION)
GATEWAY_API_CRD_CHANNEL ?= standard
GATEWAY_API_REQUIRED_CRDS ?= gatewayclasses.gateway.networking.k8s.io gateways.gateway.networking.k8s.io httproutes.gateway.networking.k8s.io referencegrants.gateway.networking.k8s.io
GATEWAY_CONFORMANCE_TEST_DIR ?= test/gateway/v1.6
GATEWAY_CONFORMANCE_PROFILE ?= GATEWAY-HTTP
GATEWAY_CONFORMANCE_SUPPORTED_FEATURES ?= Gateway,HTTPRoute,ReferenceGrant
GATEWAY_CONFORMANCE_REPORT ?= out/gateway-api-conformance/report.yaml
GATEWAY_CONFORMANCE_CONTACT ?= https://github.com/alibaba/higress/issues
GATEWAY_CONFORMANCE_RUN_TEST ?=
GATEWAY_CONFORMANCE_ALLOW_CRDS_MISMATCH ?= false
GATEWAY_CONFORMANCE_SUPPORTS_TEST_CLEANUP ?= true
GATEWAY_CONFORMANCE_CLEANUP_TEST_RESOURCES ?= true
GATEWAY_API_TEST_NAMESPACE ?= gateway-conformance-infra
GATEWAY_API_GATEWAY_SERVICE_TYPE ?= ClusterIP
GATEWAY_API_KIND_NODE_TAG ?= v1.34.0@sha256:7416a61b42b1662ca6ca89f02028ac133a309a2a30ba309614e8ec94d976dc5a
HIGRESS_CONFORMANCE_VERSION ?= $(shell git rev-parse HEAD)

INFERENCE_EXTENSION_VERSION ?= v1.4.0
INFERENCE_EXTENSION_GATEWAY_API_VERSION ?= v1.5.0
INFERENCE_EXTENSION_SOURCE_DIR ?= $(abspath out/gateway-api-inference-extension-source/$(INFERENCE_EXTENSION_VERSION))
INFERENCE_EXTENSION_REPORT ?= out/gateway-api-inference-extension/report.yaml
INFERENCE_EXTENSION_EXPECTED_PASSED ?= 12
INFERENCE_EXTENSION_CONTACT ?= @higress-group/maintainers
INFERENCE_EXTENSION_RUN_TEST ?=
INFERENCE_EXTENSION_SYSTEM_NAMESPACE ?= inference-conformance-infra
INFERENCE_EXTENSION_APP_NAMESPACE ?= inference-conformance-app-backend
INFERENCE_EXTENSION_GATEWAY_SERVICE_TYPE ?= LoadBalancer
INFERENCE_EXTENSION_GATEWAY_IMAGE_TAG ?= gie-v14-dc8a83fe-gofilter
INFERENCE_EXTENSION_GATEWAY_IMAGE_DIGEST ?= sha256:057171104acacc7381560e41491511dde0f919d44afe3860b7c8654736b98957
INFERENCE_EXTENSION_KIND_NODE_TAG ?= $(GATEWAY_API_KIND_NODE_TAG)
INFERENCE_EXTENSION_METALLB_VERSION ?= v0.13.7

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

export ENVOY_PACKAGE_URL_PATTERN?=https://github.com/higress-group/proxy/releases/download/v2.2.4/envoy-symbol-ARCH.tar.gz

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

.PHONY: build-mcp-server-wasmplugin
build-mcp-server-wasmplugin:
	PLUGIN_TYPE=GO PLUGIN_NAME=mcp-server ./tools/hack/build-wasm-plugins.sh

pre-install:
	cp api/kubernetes/customresourcedefinitions.gen.yaml helm/core/crds

define create_ns
   kubectl get namespace | grep $(1) || kubectl create namespace $(1)
endef

install: pre-install
	cd helm/higress; helm dependency build
	helm install higress helm/higress -n higress-system --create-namespace --set 'global.local=true'

HIGRESS_LATEST_IMAGE_TAG ?= latest
ENVOY_LATEST_IMAGE_TAG ?= 3498bc95bdea417fe0ce3f3017b0015cda907908
ISTIO_LATEST_IMAGE_TAG ?= 3498bc95bdea417fe0ce3f3017b0015cda907908
TEST_ISTIO_IMAGE_TAG ?= $(ISTIO_LATEST_IMAGE_TAG)

install-dev: pre-install
	helm install higress helm/core -n higress-system --create-namespace --set 'controller.tag=$(TAG)' --set 'gateway.replicas=1' --set 'pilot.tag=$(TEST_ISTIO_IMAGE_TAG)' --set 'gateway.tag=$(ENVOY_LATEST_IMAGE_TAG)' --set 'global.local=true'

install-dev-gateway-api: pre-install
	helm install higress helm/core -n $(GATEWAY_API_TEST_NAMESPACE) --create-namespace --set 'controller.tag=$(TAG)' --set 'gateway.replicas=1' --set 'pilot.tag=$(TAG)' --set 'gateway.tag=$(ENVOY_LATEST_IMAGE_TAG)' --set 'global.local=true' --set 'global.enableGatewayAPIDeploymentController=true' --set 'gateway.service.type=$(GATEWAY_API_GATEWAY_SERVICE_TYPE)'

.PHONY: install-dev-inference-extension
install-dev-inference-extension: pre-install
	helm install higress helm/core -n $(INFERENCE_EXTENSION_SYSTEM_NAMESPACE) --create-namespace \
		--set 'controller.tag=$(TAG)' \
		--set 'gateway.replicas=1' \
		--set 'pilot.tag=$(TAG)' \
		--set 'gateway.tag=$(INFERENCE_EXTENSION_GATEWAY_IMAGE_TAG)' \
		--set 'global.local=true' \
		--set 'global.enableInferenceExtension=true' \
		--set 'gateway.service.type=$(INFERENCE_EXTENSION_GATEWAY_SERVICE_TYPE)'

install-dev-wasmplugin: build-wasmplugins pre-install
	helm install higress helm/core -n higress-system --create-namespace --set 'controller.tag=$(TAG)' --set 'gateway.replicas=1' --set 'pilot.tag=$(TEST_ISTIO_IMAGE_TAG)' --set 'gateway.tag=$(ENVOY_LATEST_IMAGE_TAG)' --set 'global.local=true'  --set 'global.volumeWasmPlugins=true' --set 'global.onlyPushRouteCluster=false'

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

# install-gateway-api-crds installs the Gateway API CRDs used by the conformance suite.
.PHONY: install-gateway-api-crds
install-gateway-api-crds:
	@for version in $(GATEWAY_API_CRD_VERSIONS); do \
		kubectl apply --server-side=true --force-conflicts \
			--field-manager="gateway-api-$${version}" \
			-f "https://github.com/kubernetes-sigs/gateway-api/releases/download/$${version}/$(GATEWAY_API_CRD_CHANNEL)-install.yaml"; \
	done
	@for crd in $(GATEWAY_API_REQUIRED_CRDS); do \
		kubectl wait --for=condition=Established "crd/$${crd}" --timeout=120s; \
	done

# create-gateway-api-cluster creates the Kubernetes cluster used by Gateway API tests.
.PHONY: create-gateway-api-cluster
create-gateway-api-cluster: $(tools/kind-gateway-api)
	KIND=$(tools/kind-gateway-api) KIND_NODE_TAG=$(GATEWAY_API_KIND_NODE_TAG) tools/hack/create-cluster.sh

# delete-gateway-api-cluster deletes the Gateway API test cluster.
.PHONY: delete-gateway-api-cluster
delete-gateway-api-cluster: $(tools/kind-gateway-api)
	$(tools/kind-gateway-api) delete cluster --name higress

# kube-load-gateway-api-images loads only the images required by the Gateway API tests.
.PHONY: kube-load-gateway-api-images
kube-load-gateway-api-images: $(tools/kind-gateway-api)
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/higress $(TAG)
	tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway $(ENVOY_LATEST_IMAGE_TAG)
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot $(TAG)
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/gateway $(ENVOY_LATEST_IMAGE_TAG)

# build-test-pilot builds the Pilot image from the checked-out Istio submodule.
.PHONY: build-test-pilot
build-test-pilot: prebuild
	TARGET_ARCH=$(TARGET_ARCH) DOCKER_TARGETS="docker.pilot" IMG_URL="$(HUB)/pilot:$(TAG)" ./tools/hack/build-istio-image.sh docker

.PHONY: build-gateway-api-pilot
build-gateway-api-pilot: build-test-pilot

# gateway-conformance-test-prepare prepares a kind cluster for Gateway API tests.
.PHONY: gateway-conformance-test-prepare
gateway-conformance-test-prepare: delete-gateway-api-cluster create-gateway-api-cluster install-gateway-api-crds docker-build build-gateway-api-pilot kube-load-gateway-api-images install-dev-gateway-api
	kubectl wait --timeout=10m -n $(GATEWAY_API_TEST_NAMESPACE) deployment/higress-controller --for=condition=Available
	kubectl wait --timeout=10m -n $(GATEWAY_API_TEST_NAMESPACE) deployment/higress-gateway --for=condition=Available
	kubectl wait --timeout=10m gatewayclass/higress --for=condition=Accepted

# run-gateway-conformance-test runs the upstream Gateway API Conformance Suite.
.PHONY: run-gateway-conformance-test
run-gateway-conformance-test:
	mkdir -p $(dir $(GATEWAY_CONFORMANCE_REPORT))
	GATEWAY_CONFORMANCE_SUPPORTED_FEATURES='$(GATEWAY_CONFORMANCE_SUPPORTED_FEATURES)' \
	GATEWAY_CONFORMANCE_PROFILE='$(GATEWAY_CONFORMANCE_PROFILE)' \
	GATEWAY_CONFORMANCE_REPORT='$(abspath $(GATEWAY_CONFORMANCE_REPORT))' \
	GATEWAY_CONFORMANCE_CONTACT='$(GATEWAY_CONFORMANCE_CONTACT)' \
	GATEWAY_CONFORMANCE_RUN_TEST='$(GATEWAY_CONFORMANCE_RUN_TEST)' \
	GATEWAY_CONFORMANCE_PARALLEL='$(GATEWAY_CONFORMANCE_PARALLEL)' \
	GATEWAY_CONFORMANCE_ALLOW_CRDS_MISMATCH='$(GATEWAY_CONFORMANCE_ALLOW_CRDS_MISMATCH)' \
	GATEWAY_CONFORMANCE_SUPPORTS_TEST_CLEANUP='$(GATEWAY_CONFORMANCE_SUPPORTS_TEST_CLEANUP)' \
	GATEWAY_CONFORMANCE_CLEANUP_TEST_RESOURCES='$(GATEWAY_CONFORMANCE_CLEANUP_TEST_RESOURCES)' \
	GATEWAY_API_VERSION='$(GATEWAY_API_VERSION)' \
	GATEWAY_CONFORMANCE_TEST_DIR='$(GATEWAY_CONFORMANCE_TEST_DIR)' \
	HIGRESS_CONFORMANCE_VERSION='$(HIGRESS_CONFORMANCE_VERSION)' \
	HIGRESS_CONFORMANCE_IMAGE='$(HUB)/higress:$(TAG)' \
	tools/hack/run-gateway-api-conformance.sh

# gateway-conformance-test runs Gateway API tests as a standard Higress integration test.
.PHONY: gateway-conformance-test
gateway-conformance-test: gateway-conformance-test-prepare run-gateway-conformance-test

# gateway-conformance-test-clean deletes the Gateway API test cluster.
.PHONY: gateway-conformance-test-clean
gateway-conformance-test-clean: delete-gateway-api-cluster

# download-inference-extension-conformance downloads an isolated, exact upstream release.
.PHONY: download-inference-extension-conformance
download-inference-extension-conformance:
	INFERENCE_EXTENSION_VERSION='$(INFERENCE_EXTENSION_VERSION)' \
	INFERENCE_EXTENSION_SOURCE_DIR='$(INFERENCE_EXTENSION_SOURCE_DIR)' \
		tools/hack/download-inference-extension-conformance.sh

# install-inference-extension-crds installs the Gateway API and InferencePool CRDs
# required by the selected Inference Extension release.
.PHONY: install-inference-extension-crds
install-inference-extension-crds: download-inference-extension-conformance
	kubectl apply --server-side=true --force-conflicts \
		--field-manager='gateway-api-$(INFERENCE_EXTENSION_GATEWAY_API_VERSION)' \
		-f 'https://github.com/kubernetes-sigs/gateway-api/releases/download/$(INFERENCE_EXTENSION_GATEWAY_API_VERSION)/standard-install.yaml'
	kubectl apply --server-side=true --force-conflicts \
		--field-manager='gateway-api-inference-extension-$(INFERENCE_EXTENSION_VERSION)' \
		-f '$(INFERENCE_EXTENSION_SOURCE_DIR)/config/crd/bases/inference.networking.k8s.io_inferencepools.yaml'
	@for crd in \
		gatewayclasses.gateway.networking.k8s.io \
		gateways.gateway.networking.k8s.io \
		httproutes.gateway.networking.k8s.io \
		inferencepools.inference.networking.k8s.io; do \
		kubectl wait --for=condition=Established "crd/$${crd}" --timeout=120s; \
	done

# install-inference-extension-istio-crds installs the Istio APIs used by the
# conformance environment into a newly-created Kind cluster. Keep this out of
# run-inference-extension-conformance-test so that rerunning against an existing
# cluster does not update its cluster-scoped Istio APIs.
.PHONY: install-inference-extension-istio-crds
install-inference-extension-istio-crds: prebuild
	kubectl apply --server-side=true --force-conflicts \
		--field-manager='higress-inference-extension' \
		-f istio/istio/manifests/charts/base/files/crd-all.gen.yaml
	kubectl wait --for=condition=Established \
		crd/destinationrules.networking.istio.io --timeout=120s

.PHONY: create-inference-extension-cluster
create-inference-extension-cluster: $(tools/kind-gateway-api)
	KIND=$(tools/kind-gateway-api) KIND_NODE_TAG=$(INFERENCE_EXTENSION_KIND_NODE_TAG) tools/hack/create-cluster.sh

.PHONY: delete-inference-extension-cluster
delete-inference-extension-cluster: $(tools/kind-gateway-api)
	$(tools/kind-gateway-api) delete cluster --name higress

.PHONY: install-inference-extension-metallb
install-inference-extension-metallb:
	METALLB_VERSION='$(INFERENCE_EXTENSION_METALLB_VERSION)' tools/hack/install-kind-metallb.sh

# build-test-pilot builds the Pilot image from the checked-out Istio submodule.
.PHONY: build-test-pilot
build-test-pilot: prebuild
	TARGET_ARCH=$(TARGET_ARCH) DOCKER_TARGETS='docker.pilot' IMG_URL='$(HUB)/pilot:$(TAG)' ./tools/hack/build-istio-image.sh docker

.PHONY: kube-load-inference-extension-images
kube-load-inference-extension-images: $(tools/kind-gateway-api)
	tools/hack/docker-pull-image.sh $(HUB)/gateway $(INFERENCE_EXTENSION_GATEWAY_IMAGE_TAG)
	@test "$$(docker image inspect '$(HUB)/gateway:$(INFERENCE_EXTENSION_GATEWAY_IMAGE_TAG)' --format '{{range .RepoDigests}}{{println .}}{{end}}' | grep -F '$(INFERENCE_EXTENSION_GATEWAY_IMAGE_DIGEST)' | wc -l | tr -d ' ')" -gt 0 || \
		{ echo 'Gateway image digest does not match $(INFERENCE_EXTENSION_GATEWAY_IMAGE_DIGEST)' >&2; exit 1; }
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh $(HUB)/higress $(TAG)
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh $(HUB)/pilot $(TAG)
	KIND=$(tools/kind-gateway-api) tools/hack/kind-load-image.sh $(HUB)/gateway $(INFERENCE_EXTENSION_GATEWAY_IMAGE_TAG)

.PHONY: setup-inference-extension-epp-tls
setup-inference-extension-epp-tls:
	kubectl create namespace $(INFERENCE_EXTENSION_APP_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	kubectl apply -f test/inference-extension/manifests/epp-tls.yaml

.PHONY: inference-extension-conformance-test-prepare
inference-extension-conformance-test-prepare: delete-inference-extension-cluster create-inference-extension-cluster install-inference-extension-metallb install-inference-extension-crds install-inference-extension-istio-crds docker-build build-test-pilot kube-load-inference-extension-images install-dev-inference-extension
	kubectl wait --timeout=10m -n $(INFERENCE_EXTENSION_SYSTEM_NAMESPACE) deployment/higress-controller --for=condition=Available
	kubectl wait --timeout=10m -n $(INFERENCE_EXTENSION_SYSTEM_NAMESPACE) deployment/higress-gateway --for=condition=Available
	kubectl wait --timeout=10m gatewayclass/higress --for=condition=Accepted
	@for attempt in $$(seq 1 120); do \
		address="$$(kubectl -n $(INFERENCE_EXTENSION_SYSTEM_NAMESPACE) get service higress-gateway -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null)"; \
		if [ -n "$${address}" ]; then echo "Higress gateway address: $${address}"; exit 0; fi; \
		sleep 1; \
	done; \
	echo 'Timed out waiting for the Higress gateway LoadBalancer address' >&2; exit 1

.PHONY: run-inference-extension-conformance-test
run-inference-extension-conformance-test: download-inference-extension-conformance setup-inference-extension-epp-tls
	INFERENCE_EXTENSION_VERSION='$(INFERENCE_EXTENSION_VERSION)' \
	INFERENCE_EXTENSION_SOURCE_DIR='$(INFERENCE_EXTENSION_SOURCE_DIR)' \
	INFERENCE_EXTENSION_REPORT='$(abspath $(INFERENCE_EXTENSION_REPORT))' \
	INFERENCE_EXTENSION_EXPECTED_PASSED='$(INFERENCE_EXTENSION_EXPECTED_PASSED)' \
	INFERENCE_EXTENSION_CONTACT='$(INFERENCE_EXTENSION_CONTACT)' \
	INFERENCE_EXTENSION_RUN_TEST='$(INFERENCE_EXTENSION_RUN_TEST)' \
	HIGRESS_CONFORMANCE_VERSION='$(HIGRESS_CONFORMANCE_VERSION)' \
		tools/hack/run-inference-extension-conformance.sh

.PHONY: inference-extension-conformance-test
inference-extension-conformance-test: inference-extension-conformance-test-prepare run-inference-extension-conformance-test

.PHONY: inference-extension-conformance-test-clean
inference-extension-conformance-test-clean: delete-inference-extension-cluster

# higress-conformance-test-prepare prepares the environment for higress conformance tests.
.PHONY: higress-conformance-test-prepare
higress-conformance-test-prepare: TEST_ISTIO_IMAGE_TAG=$(TAG)
higress-conformance-test-prepare: $(tools/kind) delete-cluster create-cluster docker-build build-test-pilot kube-load-image install-dev

# higress-conformance-test runs ingress api conformance tests.
.PHONY: higress-conformance-test
higress-conformance-test: TEST_ISTIO_IMAGE_TAG=$(TAG)
higress-conformance-test: $(tools/kind) delete-cluster create-cluster docker-build build-test-pilot kube-load-image install-dev run-higress-e2e-test delete-cluster

# higress-conformance-test-clean cleans the environment for higress conformance tests.
.PHONY: higress-conformance-test-clean
higress-conformance-test-clean: $(tools/kind) delete-cluster

# higress-wasmplugin-test-prepare prepares the environment for higress wasmplugin tests.
.PHONY: higress-wasmplugin-test-prepare
higress-wasmplugin-test-prepare: TEST_ISTIO_IMAGE_TAG=$(TAG)
higress-wasmplugin-test-prepare: $(tools/kind) delete-cluster create-cluster docker-build build-test-pilot kube-load-image install-dev-wasmplugin

# higress-wasmplugin-test-prepare-skip-docker-build prepares the environment for higress wasmplugin tests without build higress docker image.
.PHONY: higress-wasmplugin-test-prepare-skip-docker-build
higress-wasmplugin-test-prepare-skip-docker-build: $(tools/kind) delete-cluster create-cluster prebuild
	@export TAG="$(HIGRESS_LATEST_IMAGE_TAG)" && \
	$(MAKE) kube-load-image && \
	$(MAKE) install-dev-wasmplugin

# higress-wasmplugin-test runs ingress wasmplugin tests.
.PHONY: higress-wasmplugin-test
higress-wasmplugin-test: TEST_ISTIO_IMAGE_TAG=$(TAG)
higress-wasmplugin-test: $(tools/kind) delete-cluster create-cluster docker-build build-test-pilot kube-load-image install-dev-wasmplugin run-higress-e2e-test-wasmplugin delete-cluster

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
	@if [ "$(TEST_ISTIO_IMAGE_TAG)" = "$(ISTIO_LATEST_IMAGE_TAG)" ]; then \
		tools/hack/docker-pull-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot $(TEST_ISTIO_IMAGE_TAG); \
	fi
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
	tools/hack/kind-load-image.sh higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/pilot $(TEST_ISTIO_IMAGE_TAG)
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
