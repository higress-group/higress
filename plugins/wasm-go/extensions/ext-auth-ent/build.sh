#!/bin/bash

# Enterprise ext-auth variant Build Script

set -e

PLUGIN_NAME="ext-auth-ent"
VERSION=$(cat VERSION 2>/dev/null || echo "1.0.0")

echo "========================================"
echo "Building Enterprise ext-auth variant"
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
    BUILD_MODE="Go 1.24 TinyGo Compilation"
else
    BUILD_MODE="Go 1.24 Native WASM Compilation"
fi

echo "Build mode: $BUILD_MODE"
echo "Go version: $(go version)"
echo ""

# Check Go version
GO_VERSION=$(go version | awk '{print $3}')
REQUIRED_VERSION="1.24"

if [ "$GO_VERSION" != "$REQUIRED_VERSION" ]; then
    echo "Warning: Go version $GO_VERSION detected, but $REQUIRED_VERSION is recommended"
    echo "Build may fail if the version is incompatible"
    echo ""
fi

# Build wasm file
echo "Building wasm file with $BUILD_MODE..."

if [ "$1" = "tinygo" ]; then
    # Build with TinyGo
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
        main.go

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
