#!/bin/bash

# Release Script for music_project_manager Plugin
# Handles version bumping, building, and release preparation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

PLUGIN_NAME="music_project_manager"

echo -e "${BLUE}🚀 Music Project Manager Release Script${NC}"
echo "========================================"

# Function to show usage
show_usage() {
    echo "Usage: $0 [VERSION_TYPE] [VERSION]"
    echo ""
    echo "VERSION_TYPE:"
    echo "  patch    - Increment patch version (0.0.1 -> 0.0.2)"
    echo "  minor    - Increment minor version (0.0.1 -> 0.1.0)"  
    echo "  major    - Increment major version (0.0.1 -> 1.0.0)"
    echo "  custom   - Set specific version (requires VERSION argument)"
    echo ""
    echo "Examples:"
    echo "  $0 patch              # Auto-increment patch version"
    echo "  $0 minor              # Auto-increment minor version"
    echo "  $0 custom 1.2.3       # Set version to 1.2.3"
    echo "  $0                    # Interactive mode"
    exit 1
}

# Function to increment version
increment_version() {
    local version=$1
    local type=$2
    
    IFS='.' read -r major minor patch <<< "$version"
    
    case $type in
        patch)
            patch=$((patch + 1))
            ;;
        minor)
            minor=$((minor + 1))
            patch=0
            ;;
        major)
            major=$((major + 1))
            minor=0
            patch=0
            ;;
        *)
            echo -e "${RED}❌ Invalid version type: $type${NC}"
            exit 1
            ;;
    esac
    
    echo "$major.$minor.$patch"
}

# Get current version
CURRENT_VERSION=$(cat VERSION 2>/dev/null || echo "0.0.0")
echo -e "${BLUE}📋 Current version: $CURRENT_VERSION${NC}"

# Handle command line arguments
if [ $# -eq 0 ]; then
    # Interactive mode
    echo ""
    echo "Select version bump type:"
    echo "1) Patch (bug fixes)     - $CURRENT_VERSION -> $(increment_version $CURRENT_VERSION patch)"
    echo "2) Minor (new features)  - $CURRENT_VERSION -> $(increment_version $CURRENT_VERSION minor)"
    echo "3) Major (breaking)      - $CURRENT_VERSION -> $(increment_version $CURRENT_VERSION major)"
    echo "4) Custom version"
    echo "5) Exit"
    
    read -p "Enter choice [1-5]: " choice
    
    case $choice in
        1) VERSION_TYPE="patch" ;;
        2) VERSION_TYPE="minor" ;;
        3) VERSION_TYPE="major" ;;
        4) 
            read -p "Enter custom version (e.g., 1.2.3): " CUSTOM_VERSION
            if [[ ! $CUSTOM_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo -e "${RED}❌ Invalid version format. Use semantic versioning (x.y.z)${NC}"
                exit 1
            fi
            VERSION_TYPE="custom"
            ;;
        5) echo "Cancelled."; exit 0 ;;
        *) echo -e "${RED}❌ Invalid choice${NC}"; exit 1 ;;
    esac
elif [ $# -eq 1 ]; then
    VERSION_TYPE=$1
    if [ "$VERSION_TYPE" = "custom" ]; then
        echo -e "${RED}❌ Custom version type requires a version number${NC}"
        show_usage
    fi
elif [ $# -eq 2 ]; then
    VERSION_TYPE=$1
    CUSTOM_VERSION=$2
    if [ "$VERSION_TYPE" != "custom" ]; then
        echo -e "${RED}❌ Version number only allowed with 'custom' type${NC}"
        show_usage
    fi
else
    show_usage
fi

# Calculate new version
if [ "$VERSION_TYPE" = "custom" ]; then
    NEW_VERSION=$CUSTOM_VERSION
else
    NEW_VERSION=$(increment_version $CURRENT_VERSION $VERSION_TYPE)
fi

echo -e "${YELLOW}📦 New version will be: $NEW_VERSION${NC}"

# Confirmation
read -p "Proceed with release? [y/N]: " confirm
if [[ ! $confirm =~ ^[Yy]$ ]]; then
    echo "Release cancelled."
    exit 0
fi

echo ""
echo -e "${BLUE}🔧 Starting release process...${NC}"

# Step 1: Update VERSION file
echo -e "${YELLOW}1. Updating VERSION file...${NC}"
echo "$NEW_VERSION" > VERSION
echo -e "${GREEN}   ✅ VERSION file updated to $NEW_VERSION${NC}"

# Step 2: Build plugin with new version
echo -e "${YELLOW}2. Building plugin with embedded version...${NC}"
if ./build.sh; then
    echo -e "${GREEN}   ✅ Plugin built successfully${NC}"
else
    echo -e "${RED}   ❌ Build failed${NC}"
    # Revert VERSION file
    echo "$CURRENT_VERSION" > VERSION
    exit 1
fi

# Step 3: Verify version is embedded
echo -e "${YELLOW}3. Verifying version embedding...${NC}"
if strings "$PLUGIN_NAME.so" | grep -q "$NEW_VERSION"; then
    echo -e "${GREEN}   ✅ Version $NEW_VERSION successfully embedded${NC}"
else
    echo -e "${RED}   ❌ Version not properly embedded${NC}"
    # Revert VERSION file  
    echo "$CURRENT_VERSION" > VERSION
    exit 1
fi

# Step 4: Git operations (if in git repo)
if git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${YELLOW}4. Git operations...${NC}"
    
    # Check for uncommitted changes (excluding VERSION file)
    if [ -n "$(git status --porcelain | grep -v 'VERSION')" ]; then
        echo -e "${YELLOW}   ⚠️  You have uncommitted changes besides VERSION file${NC}"
        read -p "   Continue anyway? [y/N]: " git_confirm
        if [[ ! $git_confirm =~ ^[Yy]$ ]]; then
            echo "   Release cancelled."
            echo "$CURRENT_VERSION" > VERSION
            exit 0
        fi
    fi
    
    # Add VERSION file and plugin binary
    git add VERSION "$PLUGIN_NAME.so"
    
    # Commit
    git commit -m "Release v$NEW_VERSION

- Update version to $NEW_VERSION
- Rebuild plugin with embedded version
- Generated by release.sh

🤖 Generated with release automation"
    
    # Create git tag
    git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION"
    
    echo -e "${GREEN}   ✅ Git commit and tag created${NC}"
    echo -e "${BLUE}   📋 To push: git push origin main && git push origin v$NEW_VERSION${NC}"
else
    echo -e "${YELLOW}4. Not a git repository - skipping git operations${NC}"
fi

# Step 5: Copy to dolphin-agent if directory exists
AGENT_PLUGINS_DIR="../dolphin-agent/uploaded_plugins"
if [ -d "$AGENT_PLUGINS_DIR" ]; then
    echo -e "${YELLOW}5. Updating dolphin-agent...${NC}"
    cp "$PLUGIN_NAME.so" "$AGENT_PLUGINS_DIR/"
    echo -e "${GREEN}   ✅ Updated dolphin-agent: $AGENT_PLUGINS_DIR/$PLUGIN_NAME.so${NC}"
fi

# Step 6: Summary
echo ""
echo -e "${GREEN}🎉 Release completed successfully!${NC}"
echo "========================================"
echo -e "${BLUE}📦 Version: $CURRENT_VERSION → $NEW_VERSION${NC}"
echo -e "${BLUE}🏷️  Plugin: $PLUGIN_NAME.so (with embedded version)${NC}"

# Show next steps
echo ""
echo -e "${YELLOW}📋 Next steps:${NC}"
if git rev-parse --git-dir > /dev/null 2>&1; then
    echo "   • Push changes: git push origin main"
    echo "   • Push tag: git push origin v$NEW_VERSION"
fi
echo "   • Test the new plugin version"
echo "   • Update documentation if needed"
echo ""

# Show final verification
echo -e "${BLUE}🔍 Verification:${NC}"
echo "   Plugin version: $(strings "$PLUGIN_NAME.so" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+$' | head -1)"
echo "   VERSION file: $(cat VERSION)"
echo ""

echo -e "${GREEN}✨ Release process complete!${NC}"
