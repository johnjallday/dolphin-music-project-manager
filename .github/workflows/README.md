# Music Project Manager CI/CD

This directory contains GitHub Actions workflows for the music-project-manager plugin.

## Workflows

### ci.yml - Continuous Integration

**Triggers:**
- Push to `main`, `master`, or `develop` branches
- Pull requests to these branches

**Jobs:**
1. **lint** - Code linting using shared template
   - Runs `go fmt`, `go vet`, `golangci-lint`

2. **plugin-test** - Plugin-specific testing
   - Validates plugin structure
   - Builds plugin binary
   - Checks for plugin interfaces

3. **test-unit** - Unit tests with coverage
   - Runs Go unit tests
   - Uploads coverage to Codecov (if token configured)

4. **build** - Multi-platform builds
   - Linux (amd64)
   - macOS (amd64, arm64)
   - Uploads artifacts

5. **security** - Security scanning
   - Runs Gosec security scanner

### release.yml - Release Automation

**Triggers:**
- Tags matching `v*.*.*` (e.g., v1.0.0, v2.1.3)

**What it does:**
- Runs tests
- Builds binaries for multiple platforms
- Creates GitHub release
- Uploads binaries and checksums

## Shared Templates

All workflows use shared templates from `../../.github-templates/`:
- `go-lint.yml` - Linting
- `go-test.yml` - Testing
- `plugin-test.yml` - Plugin validation
- `go-build.yml` - Binary builds
- `security-scan.yml` - Security scanning
- `go-release.yml` - Release creation

## Creating a Release

```bash
# Tag a new version
git tag v1.0.0

# Push the tag
git push origin v1.0.0

# GitHub Actions will automatically:
# - Build binaries
# - Create release
# - Upload assets
```

## Local Testing

Test the plugin locally before pushing:

```bash
# From workspace root
cd /Users/jjdev/Projects/ori

# Test the plugin
./scripts/ci-cd/test-plugin.sh plugins/music-project-manager

# Build the plugin
cd plugins/music-project-manager
../../scripts/ci-cd/build-go-binary.sh music-project-manager . linux/amd64,darwin/arm64
```

## Required Secrets

Configure these in GitHub repository settings:

- `CODECOV_TOKEN` (optional) - For coverage reporting
- `GITHUB_TOKEN` (automatic) - For releases

## Template Path

Plugins use `../../` to reference templates (two levels up):

```yaml
uses: ../../.github-templates/go-lint.yml@main
```

This resolves to `/Users/jjdev/Projects/ori/.github-templates/go-lint.yml`

## Troubleshooting

### Template Not Found

Ensure templates exist at workspace root:
```bash
ls ../../.github-templates/
```

### Build Failures

Check Go version matches workspace:
```bash
go version  # Should be 1.24+
```

### Plugin Binary Not Built

Ensure main.go exists in plugin root:
```bash
ls -la main.go
```

## Related Documentation

- Template README: `../../.github-templates/README.md`
- Scripts README: `../../scripts/ci-cd/README.md`
- CI/CD Plan: `../../CICD_PLAN.md`
