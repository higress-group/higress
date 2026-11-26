#!/bin/bash

# AI Adapter Plugin Build Script

set -e

PLUGIN_NAME="ai-adapter"
VERSION=$(cat VERSION 2>/dev/null || echo "1.0.0")

echo "========================================"
echo "Building AI Adapter Plugin"
echo "========================================"
echo "Plugin: $PLUGIN_NAME"
echo "Version: $VERSION"
echo ""

# Get the directory of this script
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Check if VERSION file exists, if not create it
if [ ! -f "VERSION" ]; then
    echo "1.0.0" > VERSION
    echo "Created VERSION file with default version"
    VERSION="1.0.0"
fi

# Detect build mode
if [ "$1" = "tinygo" ]; then
    BUILD_MODE="TinyGo Compilation"
else
    BUILD_MODE="Go 1.24 Native WASM Compilation"
fi

echo "Build mode: $BUILD_MODE"
echo "Go version: $(go version)"
echo ""

# Check Go version
GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
REQUIRED_VERSION="1.24"

if [[ "$GO_VERSION" < "$REQUIRED_VERSION" ]]; then
    echo "Warning: Go version $GO_VERSION detected, but $REQUIRED_VERSION is recommended"
    echo "Build may fail if version is incompatible"
    echo ""
fi

# Download dependencies if needed
echo "Downloading dependencies..."
if [ ! -f "go.sum" ] || [ ! -d "vendor" ]; then
    go mod download
    go mod tidy
fi

# Build wasm file
echo "Building wasm file with $BUILD_MODE..."

if [ "$1" = "tinygo" ]; then
    # Build with TinyGo
    if ! command -v tinygo &> /dev/null; then
        echo "Error: tinygo not found. Please install tinygo first."
        echo "Visit: https://tinygo.org/getting-started/install/"
        exit 1
    fi

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

    if [ $? -eq 0 ]; then
        echo "✅ Build successful with TinyGo"
    else
        echo "❌ Build failed with TinyGo"
        exit 1
    fi
else
    # Build with Go 1.24 native compiler
    GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o main.wasm ./

    if [ $? -eq 0 ]; then
        echo "✅ Build successful with Go 1.24"
    else
        echo "❌ Build failed with Go 1.24"
        exit 1
    fi
fi

# Check if wasm file was created
if [ ! -f "main.wasm" ]; then
    echo "❌ Error: main.wasm not found"
    exit 1
fi

# Get file size
FILE_SIZE=$(ls -lh main.wasm | awk '{print $5}')
echo ""
echo "========================================"
echo "Build successful!"
echo "========================================"
echo "Wasm file: main.wasm"
echo "File size: $FILE_SIZE"
echo ""

# Next steps
echo "Next steps:"
echo "1. Build Docker image:"
echo "   docker build -t $PLUGIN_NAME:$VERSION ."
echo ""
echo "2. Push to registry:"
echo "   docker tag $PLUGIN_NAME:$VERSION your-registry/$PLUGIN_NAME:$VERSION"
echo "   docker push your-registry/$PLUGIN_NAME:$VERSION"
echo ""
echo "3. Configure in Higress:"
echo "   Use image: your-registry/$PLUGIN_NAME:$VERSION"
echo ""
echo "4. Run tests:"
echo "   go test -v ./..."
