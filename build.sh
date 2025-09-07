#!/bin/bash

# Build script for music_project_manager plugin
# Embeds version information directly into the .so file

set -e

# Get version from VERSION file
VERSION=$(cat VERSION 2>/dev/null || echo "0.0.1")

# Build timestamp
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S_UTC')

# Git commit hash (if available)
GIT_COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

echo "Building music_project_manager plugin..."
echo "Version: $VERSION"
echo "Build Time: $BUILD_TIME" 
echo "Git Commit: $GIT_COMMIT"

# Build with version info embedded
go build -buildmode=plugin \
    -ldflags "-X main.Version=$VERSION -X main.BuildTime=$BUILD_TIME -X main.GitCommit=$GIT_COMMIT" \
    -o music_project_manager.so .

echo "✅ Plugin built successfully: music_project_manager.so"
echo "🏷️  Version $VERSION embedded in binary"
