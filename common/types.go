package common

import (
	"context"

	"github.com/openai/openai-go/v2"
)

// PluginTool is the interface that plugins must implement to be used as tools.
type PluginTool interface {
	// Definition returns the function definition for OpenAI function calling.
	Definition() openai.FunctionDefinitionParam
	// Call executes the tool logic with the given arguments JSON string and returns the result JSON string.
	Call(ctx context.Context, args string) (string, error)
}

// AgentContext provides information about the current agent to plugins.
type AgentContext struct {
	// Name is the name of the current agent (e.g., "reaper-project-manager", "default")
	Name string
	// ConfigPath is the path to the agent's main config file (agents/{name}/config.json)
	ConfigPath string
	// SettingsPath is the path to the agent's settings file (agents/{name}/agent_settings.json)
	SettingsPath string
	// AgentDir is the path to the agent's directory (agents/{name}/)
	AgentDir string
}

// AgentAwareTool extends PluginTool with agent context information.
// Plugins can optionally implement this interface to receive current agent info.
type AgentAwareTool interface {
	PluginTool
	// SetAgentContext provides the current agent information to the plugin
	SetAgentContext(ctx AgentContext)
}

// Settings represents the plugin configuration
type Settings struct {
	DefaultTemplate string `json:"default_template"`
	ProjectDir      string `json:"project_dir"`
	TemplateDir     string `json:"template_dir"`
	Initialized     bool   `json:"initialized"`
}

// IndividualAgentConfig represents the structure of an individual agent file
type IndividualAgentConfig struct {
	Settings AgentSettings           `json:"Settings"`
	Plugins  map[string]PluginConfig `json:"Plugins"`
}

// AgentsIndexConfig represents the main agents.json structure (for finding current agent)
type AgentsIndexConfig struct {
	Agents  map[string]interface{} `json:"agents"` // We only need to read the current agent name
	Current string                 `json:"current"`
}

type AgentSettings struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
}

type PluginConfig struct {
	Definition PluginDefinition `json:"Definition"`
	Path       string           `json:"Path"`
	Version    string           `json:"Version"`
}

type PluginDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// AgentInfo represents an agent in the main agents.json file
type AgentInfo struct {
	Settings AgentSettings           `json:"Settings"`
	Plugins  map[string]PluginConfig `json:"Plugins"`
}

// AgentsConfig represents the complete agents.json structure (legacy format)
type AgentsConfig struct {
	Agents  map[string]AgentInfo `json:"agents"`
	Current string               `json:"current"`
}