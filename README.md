# Music Project Manager Plugin

A standalone Go plugin for managing music projects with REAPER DAW integration.

## Features

- **Create Projects**: Create new music projects with custom BPM settings
- **Directory Management**: Configure project and template directories
- **Template System**: Use .RPP template files to bootstrap new projects
- **Cross-Platform**: Works on macOS, Windows, and Linux with appropriate REAPER paths
- **Agent Context**: Supports agent-based configuration for multi-user environments

## Building

```bash
./build.sh
```

This will create `music_project_manager.so` plugin file.

## Operations

### Setup Operations
- `init_setup`: Get setup guidance with platform-specific suggested directories
- `complete_setup`: Complete initial setup with project and template directories
- `set_project_dir`: Set the directory where new projects will be created
- `set_template_dir`: Set the directory containing .RPP template files
- `get_settings`: View current configuration

### Project Operations
- `create_project`: Create and launch a new music project
  - `name` (required): Project name
  - `bpm` (optional): BPM for the project (default: uses template BPM)

## Usage Example

```json
{
  "operation": "create_project",
  "name": "MyNewSong",
  "bpm": 140
}
```

## Configuration

The plugin stores settings in agent-specific configuration files:
- `config.json`: Plugin definition and schema
- `agent_settings.json`: User-specific directory paths and settings

## Platform Support

### Default Template Directories
- **macOS**: `~/Library/Application Support/REAPER/ProjectTemplates`
- **Windows**: `~/AppData/Roaming/REAPER/ProjectTemplates`  
- **Linux**: `~/Music/Templates`

### Project Directory
- **All platforms**: `~/Music/Projects`

## Development

This plugin implements the following interfaces:
- `PluginTool`: Basic plugin functionality
- `AgentAwareTool`: Agent context support for multi-agent environments

## Dependencies

- Go 1.24+
- github.com/openai/openai-go/v2 (for OpenAI function definitions)
- REAPER DAW (for launching projects)

## License

This project is part of the Dolphin Agent ecosystem.