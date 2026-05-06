#!/bin/bash

set -euo pipefail

mkdir -p dist

# Extract module name from go.mod.
MODULE_NAME=$(grep '^module ' go.mod | awk '{print $2}')

# Get version from git tag, or use commit hash as fallback.
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "dev")

# Build timestamp.
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')

# Git commit hash.
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Build flags to inject version info.
LDFLAGS="-X 'main.Version=${VERSION}' -X 'main.BuildTime=${BUILD_TIME}' -X 'main.GitCommit=${GIT_COMMIT}' -X 'main.ModuleName=${MODULE_NAME}'"

echo "Building with:"
echo "  Module: ${MODULE_NAME}"
echo "  Version: ${VERSION}"
echo "  Commit: ${GIT_COMMIT}"
echo "  Build Time: ${BUILD_TIME}"
echo ""

echo "Building for Linux AMD64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o "dist/ts-proxy-${VERSION}-linux-amd64" .

echo "Building for macOS ARM64..."
GOOS=darwin GOARCH=arm64 go build -ldflags "${LDFLAGS}" -o "dist/ts-proxy-${VERSION}-mac-arm64" .

echo "Building for macOS AMD64..."
GOOS=darwin GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o "dist/ts-proxy-${VERSION}-mac-amd64" .

echo "Building for Windows AMD64..."
GOOS=windows GOARCH=amd64 go build -ldflags "${LDFLAGS}" -o "dist/ts-proxy-${VERSION}-windows-amd64.exe" .

echo ""
echo "Build complete! Binaries created:"
ls -lh dist/
