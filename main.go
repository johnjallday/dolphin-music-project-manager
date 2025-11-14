package main

import (
	_ "embed"

	"github.com/hashicorp/go-plugin"
	"github.com/johnjallday/music_project_manager/internal/tool"
	"github.com/johnjallday/ori-agent/pluginapi"
)

//go:embed plugin.yaml
var configYAML string

func main() {
	// Parse plugin config from embedded YAML
	config := pluginapi.ReadPluginConfig(configYAML)

	// Create music project manager tool with base plugin
	musicTool := tool.NewMusicProjectManagerTool(
		"ori-music-project-manager",       // Plugin name
		config.Version,                    // Version from config
		config.Requirements.MinOriVersion, // Min agent version
		"v1",                              // API version
	)

	// Set metadata from config
	if metadata, err := config.ToMetadata(); err == nil {
		musicTool.SetMetadata(metadata)
	}

	// Serve the plugin directly
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.Handshake,
		Plugins: map[string]plugin.Plugin{
			"tool": &pluginapi.ToolRPCPlugin{Impl: musicTool},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
