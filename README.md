# 🎵 Ori Music Project Manager

A powerful plugin for managing REAPER DAW music projects with ori-agent integration.

![REAPER](https://img.shields.io/badge/REAPER-Compatible-ff6b35)
![Go](https://img.shields.io/badge/Go-1.25-00add8)
![Plugin](https://img.shields.io/badge/Plugin-Ori%20Agent-6366f1)
![License](https://img.shields.io/badge/License-MIT-green)

## 🎯 Features

- **Create Projects**: Generate new REAPER projects with custom BPM settings from templates
- **Smart Search**: List and filter projects by name or BPM range
- **Quick Access**: Open projects in REAPER or reveal them in Finder
- **Project Scanning**: Automatically scan directories for .RPP files with BPM detection
- **Rename Projects**: Safely rename project folders and files with automatic updates
- **Structured Results**: Beautiful table displays for project listings

## 📥 Installation

### Download Pre-built Binaries

1. Go to [Releases](https://github.com/johnjallday/ori-music-project-manager/releases)
2. Download the binary for your platform:
   - macOS Intel: `ori-music-project-manager-darwin-amd64`
   - macOS Apple Silicon: `ori-music-project-manager-darwin-arm64`
3. Make it executable:
   ```bash
   chmod +x ori-music-project-manager-darwin-arm64
   ```
4. Move to your ori-agent plugins directory
5. Configure in ori-agent settings

### Build from Source

```bash
./build.sh
```

This creates `ori-music-project-manager` executable with version embedded from `plugin.yaml`.

## 🚀 Operations

### Project Management

#### `create_project`
Create and launch a new REAPER project from template
```json
{
  "operation": "create_project",
  "name": "MyNewSong",
  "bpm": 140
}
```

#### `scan`
Scan project directory for .RPP files (runs in background)
```json
{
  "operation": "scan"
}
```

#### `list_projects`
Display 30 most recent projects in a table
```json
{
  "operation": "list_projects"
}
```

#### `filter_project`
Filter projects by name and/or BPM
```json
{
  "operation": "filter_project",
  "name": "beats",
  "bpm": 140,
  "min_bpm": 120,
  "max_bpm": 150
}
```

#### `open_project`
Open a project in REAPER DAW
```json
{
  "operation": "open_project",
  "path": "/Users/name/Music/Projects/MySong/MySong.RPP"
}
```

#### `open_in_finder`
Reveal project in Finder (by path or name)
```json
{
  "operation": "open_in_finder",
  "name": "MySong"
}
```

#### `rename_project`
Rename a project folder and file
```json
{
  "operation": "rename_project",
  "name": "OldName",
  "new_name": "NewName"
}
```

## ⚙️ Configuration

The plugin uses ori-agent's configuration system. Configure these settings:

- **project_dir**: Directory where projects are stored (default: `~/Music/Projects`)
- **template_dir**: Directory containing REAPER templates (default: `~/Library/Application Support/REAPER/ProjectTemplates`)
- **default_template**: Path to default .RPP template file

## 🏗️ Architecture

```
ori-music-project-manager/
├── internal/
│   ├── tool/           # Core plugin implementation
│   │   └── tool.go     # All business logic
│   └── types/          # Type definitions
│       └── types.go    # Shared types
├── main.go             # Plugin entry point
├── plugin.yaml         # Plugin metadata
└── build.sh            # Build script
```

## 🔧 Development

### Prerequisites

- Go 1.25+
- REAPER DAW
- ori-agent (for testing)

### Project Structure

This plugin follows Go best practices:
- **Clean separation**: Main file is minimal (40 lines)
- **Internal packages**: Core logic in `internal/tool` and `internal/types`
- **Provider-agnostic**: Works with any LLM provider (OpenAI, Claude, etc.)

### Implemented Interfaces

- `pluginapi.PluginTool`: Core plugin functionality
- `pluginapi.PluginCompatibility`: Version compatibility
- `pluginapi.MetadataProvider`: Plugin metadata
- `pluginapi.DefaultSettingsProvider`: Default configuration
- `pluginapi.InitializationProvider`: Configuration initialization
- `pluginapi.AgentAwareTool`: Agent context awareness

### Building

The build process:
1. Reads version from `plugin.yaml`
2. Embeds version info in binary
3. Creates executable for current platform

### Release Process

1. Update version in `plugin.yaml`:
   ```yaml
   version: 0.0.8
   ```

2. Create and push a git tag:
   ```bash
   git tag v0.0.8
   git push origin v0.0.8
   ```

3. GitHub Actions automatically:
   - Reads platforms from `plugin.yaml`
   - Builds binaries for all specified platforms
   - Runs tests
   - Generates checksums
   - Creates GitHub release with artifacts

## 📋 Platform Support

Currently supports:
- **macOS**: Intel (amd64) and Apple Silicon (arm64)

To add more platforms, update `plugin.yaml`:
```yaml
platforms:
  - os: darwin
    architectures: [amd64, arm64]
  - os: linux
    architectures: [amd64, arm64]
  - os: windows
    architectures: [amd64]
```

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `go test ./...`
5. Submit a pull request

## 📄 License

MIT License - see [LICENSE](LICENSE) file for details

## 🔗 Links

- [ori-agent](https://github.com/johnjallday/ori-agent)
- [REAPER](https://www.reaper.fm/)
- [Issues](https://github.com/johnjallday/ori-music-project-manager/issues)

---

**Made with ❤️ for the REAPER and AI community**
