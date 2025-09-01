#!/bin/bash

# Release script for music-project-manager plugin
# Creates a GitHub release with the built .so file

set -e # Exit on any error

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PLUGIN_NAME="music-project-manager"
OUTPUT_NAME="music_project_manager.so"

# Read version from VERSION file
if [ ! -f "VERSION" ]; then
  echo -e "${RED}✗ VERSION file not found${NC}"
  exit 1
fi

VERSION=$(cat VERSION | tr -d '\n\r')
TAG="v$VERSION"

echo -e "${BLUE}Creating release for music-project-manager $TAG...${NC}"

# Check if we're in a git repository
if ! git rev-parse --git-dir >/dev/null 2>&1; then
  echo -e "${RED}✗ Not in a git repository${NC}"
  exit 1
fi

# Check if there are uncommitted changes
if ! git diff-index --quiet HEAD --; then
  echo -e "${RED}✗ There are uncommitted changes. Please commit or stash them first.${NC}"
  exit 1
fi

# Build the plugin
echo -e "${YELLOW}Building plugin...${NC}"
if ! ./build.sh; then
  echo -e "${RED}✗ Build failed${NC}"
  exit 1
fi

# Verify the .so file exists
if [ ! -f "$OUTPUT_NAME" ]; then
  echo -e "${RED}✗ Built plugin file $OUTPUT_NAME not found${NC}"
  exit 1
fi

# Check if tag already exists
if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo -e "${YELLOW}⚠ Tag $TAG already exists. Deleting existing tag and release...${NC}"
  git tag -d "$TAG" || true
  if git remote get-url origin >/dev/null 2>&1; then
    git push origin ":refs/tags/$TAG" || true
  fi
  gh release delete "$TAG" --yes || true
fi

# Create git tag
echo -e "${YELLOW}Creating git tag $TAG...${NC}"
git tag "$TAG"
if git remote get-url origin >/dev/null 2>&1; then
  git push origin "$TAG"
else
  echo -e "${YELLOW}⚠ No origin remote found, skipping git push${NC}"
fi

# Create GitHub release with the .so file
echo -e "${YELLOW}Creating GitHub release...${NC}"
gh release create "$TAG" \
  --title "Release $TAG" \
  --notes "Release $TAG of music-project-manager plugin

## Installation
Download the \`$OUTPUT_NAME\` file and place it in your plugins directory.

## Changes
- Version $VERSION release" \
  "$OUTPUT_NAME"

echo -e "${GREEN}🎉 Release $TAG created successfully!${NC}"
echo -e "${BLUE}Plugin file $OUTPUT_NAME has been uploaded to the release.${NC}"
echo -e "${BLUE}Users can download it from: https://github.com/johnjallday/dolphin-music-project-manager/releases/tag/$TAG${NC}"
