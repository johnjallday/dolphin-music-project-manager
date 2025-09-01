#!/bin/bash

# Build script for music_project_manager plugin

echo "Building music_project_manager plugin..."

# Build the plugin
go build -buildmode=plugin -o music_project_manager.so music_project_manager.go

if [ $? -eq 0 ]; then
    echo "✓ Successfully built music_project_manager.so"
    echo "Plugin binary: music_project_manager.so"
    ls -la music_project_manager.so
else
    echo "✗ Failed to build music_project_manager"
    exit 1
fi