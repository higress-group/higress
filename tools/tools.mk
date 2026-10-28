tools.bindir = tools/bin
tools.srcdir = tools/src

ifeq ($(origin GOOS), undefined)
	GOOS := $(shell go env GOOS)
endif
ifeq ($(origin GOARCH), undefined)
	GOARCH := $(shell go env GOARCH)
endif


# Prebuilt tools
# ==============
#
tools/golangci-lint = $(tools.bindir)/golangci-lint
GOLANGCI_LINT_VERSION = 1.57.2
GOLANGCI_LINT_ARCHIVE = golangci-lint-$(GOLANGCI_LINT_VERSION)-$(GOOS)-$(GOARCH).tar.gz
GOLANGCI_LINT_URL = https://github.com/golangci/golangci-lint/releases/download/v$(GOLANGCI_LINT_VERSION)/$(GOLANGCI_LINT_ARCHIVE)
GOLANGCI_LINT_SHA256_darwin_amd64 = 83157f5378d259d51c88e310e88513bd80bd42e497974a1bbe51b82931f229c7
GOLANGCI_LINT_SHA256_darwin_arm64 = 0d6f10544fc0b5bd94b9eeb20d89646b9c19b52a98dca1ca62f94c08ac641b98
GOLANGCI_LINT_SHA256_linux_amd64 = 391483daa5d58d037832ba2fac709fdb5df0c67471c0d7698d1f67cbfa5f10f0
GOLANGCI_LINT_SHA256_linux_arm64 = 9e079e19b3d81e357d2acc90518c6a86a533e26a74af1e5eadf8dfcd640b66ee
GOLANGCI_LINT_SHA256 = $(GOLANGCI_LINT_SHA256_$(GOOS)_$(GOARCH))
ifeq ($(GOOS),darwin)
GOLANGCI_LINT_SHA256_CMD = shasum -a 256
else
GOLANGCI_LINT_SHA256_CMD = sha256sum
endif
$(tools.bindir)/$(GOLANGCI_LINT_ARCHIVE):
	mkdir -p $(@D)
	test -n "$(GOLANGCI_LINT_SHA256)" || (echo "Unsupported golangci-lint platform: $(GOOS)/$(GOARCH)"; exit 1)
	curl -sfL $(GOLANGCI_LINT_URL) -o $@ || (rm -f $@; exit 1)
	echo "$(GOLANGCI_LINT_SHA256)  $@" | $(GOLANGCI_LINT_SHA256_CMD) -c - || (rm -f $@; exit 1)
$(tools/golangci-lint): $(tools.bindir)/$(GOLANGCI_LINT_ARCHIVE)
	tar -C $(@D) -zxmf $< --strip-components=1 golangci-lint-$(GOLANGCI_LINT_VERSION)-$(GOOS)-$(GOARCH)/golangci-lint

# `go get`-able things
# ====================
#
tools/controller-gen = $(tools.bindir)/controller-gen
tools/kustomize      = $(tools.bindir)/kustomize
tools/kind           = $(tools.bindir)/kind
tools/kind-gateway-api = $(tools.bindir)/kind-gateway-api
tools/setup-envtest  = $(tools.bindir)/setup-envtest
$(tools.bindir)/%: $(tools.srcdir)/%/pin.go $(tools.srcdir)/%/go.mod
	cd $(<D) && GOOS= GOARCH= go build -o $(abspath $@) $$(sed -En 's,^import "(.*)".*,\1,p' pin.go)

# `pip install`-able things
# =========================
#
tools/codespell    = $(tools.bindir)/codespell
tools/yamllint     = $(tools.bindir)/yamllint
$(tools.bindir)/%.d/venv: $(tools.srcdir)/%/requirements.txt
	mkdir -p $(@D)
	python3 -m venv $@
	$@/bin/pip3 install -r $< || (rm -rf $@; exit 1)
$(tools.bindir)/%: $(tools.bindir)/%.d/venv
	ln -sf $*.d/venv/bin/$* $@

ifneq ($(GOOS),windows)
# Shellcheck
# ==========
#
tools/shellcheck = $(tools.bindir)/shellcheck
SHELLCHECK_VERSION=0.8.0
SHELLCHECK_ARCH=$(shell uname -m)
# shellcheck uses the same binary on Intel and Apple Silicon Mac.
ifeq ($(GOOS),darwin)
SHELLCHECK_ARCH=x86_64
endif
SHELLCHECK_TXZ = https://github.com/koalaman/shellcheck/releases/download/v$(SHELLCHECK_VERSION)/shellcheck-v$(SHELLCHECK_VERSION).$(GOOS).$(SHELLCHECK_ARCH).tar.xz
tools/bin/$(notdir $(SHELLCHECK_TXZ)):
	mkdir -p $(@D)
	curl -sfL $(SHELLCHECK_TXZ) -o $@
%/bin/shellcheck: %/bin/$(notdir $(SHELLCHECK_TXZ))
	mkdir -p $(@D)
	tar -C $(@D) -Jxmf $< --strip-components=1 shellcheck-v$(SHELLCHECK_VERSION)/shellcheck
endif
