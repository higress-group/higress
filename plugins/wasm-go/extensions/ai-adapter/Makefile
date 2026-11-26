.PHONY: build build-go build-tinygo test clean docker-build docker-push

PLUGIN_NAME := ai-adapter
VERSION := $(shell cat VERSION 2>/dev/null || echo "1.0.0")
REGISTRY ?= your-registry

# Build with Go 1.24 native compiler
build-go:
	@echo "Building with Go 1.24 native compiler..."
	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./
	@echo "Build successful: main.wasm"

# Build with TinyGo
build-tinygo:
	@echo "Building with TinyGo..."
	tinygo build -o main.wasm -target=wasi \
		-scheduler=none \
		-no-stack-overflow-check=true \
		-panic=trap \
		-gc=leaking \
		-deadcode \
		-map \
		-heap-size-bytes=131072 \
		-optimize=-z \
		-optimise-size \
		-no-strip \
		-trim-path \
		-stack-size-bytes=8192 \
		-dump=source \
		./
	@echo "Build successful: main.wasm"

# Default build (use Go 1.24)
build: build-go

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -f main.wasm
	rm -f main.h
	go clean
	@echo "Clean complete"

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(PLUGIN_NAME):$(VERSION) .
	docker tag $(PLUGIN_NAME):$(VERSION) $(REGISTRY)/$(PLUGIN_NAME):$(VERSION)
	@echo "Docker image built: $(REGISTRY)/$(PLUGIN_NAME):$(VERSION)"

# Push Docker image
docker-push:
	@echo "Pushing Docker image..."
	docker push $(REGISTRY)/$(PLUGIN_NAME):$(VERSION)
	@echo "Docker image pushed: $(REGISTRY)/$(PLUGIN_NAME):$(VERSION)"

# Build and push
all: build docker-build docker-push
