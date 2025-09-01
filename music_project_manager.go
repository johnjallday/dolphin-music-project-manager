package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

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

// SettingsManager manages plugin settings
type SettingsManager struct {
	settings *Settings
}

var globalSettings = &SettingsManager{}
var globalAgentContext *AgentContext

// musicProjectManagerTool implements Tool for music project management.
type musicProjectManagerTool struct {
	agentContext *AgentContext
}

// Definition returns the OpenAI function definition for music project management operations.
func (m *musicProjectManagerTool) Definition() openai.FunctionDefinitionParam {
	return openai.FunctionDefinitionParam{
		Name:        "music_project_manager",
		Description: openai.String("Manage music projects: create projects, configure settings, and setup plugin"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "Operation to perform",
					"enum":        []string{"create_project", "set_project_dir", "set_template_dir", "get_settings", "init_setup", "complete_setup"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Project name (required for create_project)",
				},
				"bpm": map[string]any{
					"type":        "integer",
					"description": "BPM for the project (optional for create_project)",
					"minimum":     60,
					"maximum":     300,
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Directory path (required for set_project_dir, set_template_dir)",
				},
				"project_dir": map[string]any{
					"type":        "string",
					"description": "Project directory path (required for complete_setup)",
				},
				"template_dir": map[string]any{
					"type":        "string",
					"description": "Template directory path (required for complete_setup)",
				},
			},
			"required": []string{"operation"},
		},
	}
}

// Call is invoked with the function arguments and dispatches to the appropriate operation.
func (m *musicProjectManagerTool) Call(ctx context.Context, args string) (string, error) {
	var p struct {
		Operation   string `json:"operation"`
		Name        string `json:"name"`
		BPM         int    `json:"bpm"`
		Path        string `json:"path"`
		ProjectDir  string `json:"project_dir"`
		TemplateDir string `json:"template_dir"`
	}

	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	switch p.Operation {
	case "create_project":
		if p.Name == "" {
			return "", fmt.Errorf("name is required for create_project operation")
		}
		return m.handleCreateProject(p.Name, p.BPM)

	case "set_project_dir":
		if p.Path == "" {
			return "", fmt.Errorf("path is required for set_project_dir operation")
		}
		return m.handleSetProjectDir(p.Path)

	case "set_template_dir":
		if p.Path == "" {
			return "", fmt.Errorf("path is required for set_template_dir operation")
		}
		return m.handleSetTemplateDir(p.Path)

	case "get_settings":
		return m.handleGetSettings()

	case "init_setup":
		return m.handleInitSetup()

	case "complete_setup":
		if p.ProjectDir == "" || p.TemplateDir == "" {
			return "", fmt.Errorf("both project_dir and template_dir are required for complete_setup operation")
		}
		return m.handleCompleteSetup(p.ProjectDir, p.TemplateDir)

	default:
		return "", fmt.Errorf("unknown operation %q", p.Operation)
	}
}

// handleCreateProject creates a new music project
func (m *musicProjectManagerTool) handleCreateProject(name string, bpm int) (string, error) {
	// Get current settings from agent_settings.json
	agentSettings, err := m.getAgentSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load agent settings: %w", err)
	}

	// Check if settings are initialized
	if len(agentSettings) == 0 {
		return "Music Project Manager needs to be set up first. Please run music_project_manager with operation 'init_setup' to begin the setup process.", nil
	}

	// Get required settings
	projectDirInterface, hasProjectDir := agentSettings["project_dir"]
	templateDirInterface, hasTemplateDir := agentSettings["template_dir"]
	
	if !hasProjectDir || !hasTemplateDir {
		return "Music Project Manager needs to be set up first. Please configure project_dir and template_dir using 'set_project_dir' and 'set_template_dir' operations.", nil
	}

	projectDirBase, ok := projectDirInterface.(string)
	if !ok || projectDirBase == "" {
		return "", fmt.Errorf("project directory not configured")
	}

	templateDir, ok := templateDirInterface.(string)
	if !ok || templateDir == "" {
		return "", fmt.Errorf("template directory not configured")
	}

	// Construct default template path
	defaultTemplate := filepath.Join(templateDir, "default.RPP")

	// Make the project folder
	projectDir := filepath.Join(projectDirBase, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory: %w", err)
	}

	// Copy the .RPP
	dest := filepath.Join(projectDir, name+".RPP")
	data, err := os.ReadFile(defaultTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to read template file: %w", err)
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write project file: %w", err)
	}

	// Patch TEMPO if needed
	if bpm > 0 {
		content, err := os.ReadFile(dest)
		if err != nil {
			return "", fmt.Errorf("failed to read project file for BPM update: %w", err)
		}
		lines := strings.Split(string(content), "\n")
		for i, line := range lines {
			trimmed := strings.TrimLeft(line, " \t")
			if strings.HasPrefix(trimmed, "TEMPO ") {
				indent := line[:len(line)-len(trimmed)]
				parts := strings.Fields(trimmed)
				if len(parts) >= 2 {
					parts[1] = strconv.Itoa(bpm)
					lines[i] = indent + strings.Join(parts, " ")
				}
				break
			}
		}
		if err := os.WriteFile(dest, []byte(strings.Join(lines, "\n")), 0644); err != nil {
			return "", fmt.Errorf("failed to update BPM in project file: %w", err)
		}
	}

	// Launch Reaper
	cmd := exec.Command("open", "-a", "Reaper", dest)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to launch Reaper: %w", err)
	}

	msg := fmt.Sprintf("Created and launched project: %s", dest)
	if bpm > 0 {
		msg += fmt.Sprintf(" (BPM %d)", bpm)
	}
	return msg, nil
}

// handleSetProjectDir sets the project directory
func (m *musicProjectManagerTool) handleSetProjectDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	settings := globalSettings.getCurrentSettings()
	settings.ProjectDir = path
	globalSettings.settings = settings

	// Update agent settings to persist the setting
	if err := m.updateAgentSettings(path, ""); err != nil {
		// Don't fail the operation if agent settings update fails, just log it
		return fmt.Sprintf("Project directory set to: %s\n⚠️  Could not persist to agent settings: %v\n\nPlease check that the agents directory is writable and the plugin has access to it.", path, err), nil
	}

	return fmt.Sprintf("✅ Project directory set to: %s\n✅ Successfully persisted to agent settings", path), nil
}

// handleSetTemplateDir sets the template directory
func (m *musicProjectManagerTool) handleSetTemplateDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	settings := globalSettings.getCurrentSettings()
	settings.TemplateDir = path
	if settings.DefaultTemplate != "" {
		settings.DefaultTemplate = filepath.Join(path, "default.RPP")
	}
	globalSettings.settings = settings

	// Update agent settings to persist the setting
	if err := m.updateAgentSettings("", path); err != nil {
		// Don't fail the operation if agent settings update fails, just log it
		return fmt.Sprintf("Template directory set to: %s\n⚠️  Could not persist to agent settings: %v\n\nPlease check that the agents directory is writable and the plugin has access to it.", path, err), nil
	}

	return fmt.Sprintf("✅ Template directory set to: %s\n✅ Successfully persisted to agent settings", path), nil
}

// handleGetSettings returns current settings from agent_settings.json
func (m *musicProjectManagerTool) handleGetSettings() (string, error) {
	// Check both instance and global agent context
	var agentContext *AgentContext
	if m.agentContext != nil && m.agentContext.SettingsPath != "" {
		agentContext = m.agentContext
	} else if globalAgentContext != nil && globalAgentContext.SettingsPath != "" {
		agentContext = globalAgentContext
	}

	if agentContext == nil {
		// Fall back to in-memory settings if no agent context
		settings := globalSettings.getCurrentSettings()
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal settings: %w", err)
		}
		return string(data), nil
	}

	settingsFilePath := agentContext.SettingsPath

	// Read settings from the agent_settings.json file
	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return "", fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		return "", fmt.Errorf("failed to read agent settings file at %s: %w", settingsFilePath, err)
	}

	// Extract music_project_manager settings
	var musicSettings map[string]interface{}
	if ms, exists := agentSettings["music_project_manager"].(map[string]interface{}); exists {
		musicSettings = ms
	} else {
		musicSettings = make(map[string]interface{})
	}

	// Create formatted settings response
	formattedSettings := map[string]interface{}{
		"project_dir":  musicSettings["project_dir"],
		"template_dir": musicSettings["template_dir"],
		"path":         musicSettings["path"],
		"initialized":  len(musicSettings) > 0,
	}

	// Add default_template if template_dir exists
	if templateDir, ok := musicSettings["template_dir"].(string); ok && templateDir != "" {
		formattedSettings["default_template"] = filepath.Join(templateDir, "default.RPP")
	}

	data, err := json.MarshalIndent(formattedSettings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal settings: %w", err)
	}
	return string(data), nil
}

// getSuggestedDirectories returns platform-appropriate suggested directories
func getSuggestedDirectories() (projectDir, templateDir string) {
	// Use ~ for cross-platform home directory
	projectDir = "~/Music/Projects"
	
	// Detect platform for REAPER template directory
	if runtime.GOOS == "darwin" {
		// macOS - use standard REAPER application support directory
		templateDir = "~/Library/Application Support/REAPER/ProjectTemplates"
	} else if runtime.GOOS == "windows" {
		// Windows - use standard REAPER AppData directory
		templateDir = "~/AppData/Roaming/REAPER/ProjectTemplates"
	} else {
		// Linux/other - use generic location
		templateDir = "~/Music/Templates"
	}
	
	return projectDir, templateDir
}

// getAgentSettings reads the music_project_manager settings from agent_settings.json
func (m *musicProjectManagerTool) getAgentSettings() (map[string]interface{}, error) {
	// Check both instance and global agent context
	var agentContext *AgentContext
	if m.agentContext != nil && m.agentContext.SettingsPath != "" {
		agentContext = m.agentContext
	} else if globalAgentContext != nil && globalAgentContext.SettingsPath != "" {
		agentContext = globalAgentContext
	}

	if agentContext == nil {
		return nil, fmt.Errorf("no agent context available - cannot determine settings file path")
	}

	settingsFilePath := agentContext.SettingsPath

	// Read settings from the agent_settings.json file
	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return nil, fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		return nil, fmt.Errorf("failed to read agent settings file at %s: %w", settingsFilePath, err)
	}

	// Extract music_project_manager settings
	if musicSettings, exists := agentSettings["music_project_manager"].(map[string]interface{}); exists {
		return musicSettings, nil
	}

	// Return empty map if no music_project_manager settings found
	return make(map[string]interface{}), nil
}

// handleInitSetup checks setup status and provides guidance
func (m *musicProjectManagerTool) handleInitSetup() (string, error) {
	if globalSettings.IsInitialized() {
		settings := globalSettings.getCurrentSettings()
		return "Music Project Manager is already set up and ready to use.\n\nCurrent settings:\n" +
			fmt.Sprintf("- Project Directory: %s\n", settings.ProjectDir) +
			fmt.Sprintf("- Template Directory: %s\n", settings.TemplateDir) +
			fmt.Sprintf("- Default Template: %s\n", settings.DefaultTemplate) +
			"\nUse operation 'get_settings' to view detailed configuration.", nil
	}

	suggestedProjectDir, suggestedTemplateDir := getSuggestedDirectories()

	return fmt.Sprintf("🎵 Welcome to Music Project Manager! \n\nThis is your first time using the plugin. Please complete the setup by providing:\n\n"+
		"1. **Project Directory** - Where new music projects will be created\n"+
		"   Suggested: %s\n\n"+
		"2. **Template Directory** - Where your .RPP template files are stored\n"+
		"   Suggested: %s\n\n"+
		"Please use operation 'complete_setup' with project_dir and template_dir parameters to finish the setup.\n\n"+
		"Example: music_project_manager(operation=\"complete_setup\", project_dir=\"%s\", template_dir=\"%s\")",
		suggestedProjectDir, suggestedTemplateDir, suggestedProjectDir, suggestedTemplateDir), nil
}

// handleCompleteSetup completes initial setup
func (m *musicProjectManagerTool) handleCompleteSetup(projectDir, templateDir string) (string, error) {
	// Validate and create directories
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory %s: %w", projectDir, err)
	}
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create template directory %s: %w", templateDir, err)
	}

	// Create and save settings
	settings := &Settings{
		ProjectDir:      projectDir,
		TemplateDir:     templateDir,
		DefaultTemplate: filepath.Join(templateDir, "default.RPP"),
		Initialized:     true,
	}

	globalSettings.settings = settings

	// Update agents.json to persist both settings
	persistMessage := ""
	if err := m.updateAgentSettings(projectDir, templateDir); err != nil {
		persistMessage = fmt.Sprintf(" (Warning: Could not persist to agent config: %v)", err)
	} else {
		persistMessage = " and persisted to agent config"
	}

	return fmt.Sprintf("✅ Setup completed successfully!\n\n"+
		"Configuration saved%s:\n"+
		"- Project Directory: %s\n"+
		"- Template Directory: %s\n"+
		"- Default Template: %s\n\n"+
		"You can now use operation 'create_project' to create new music projects. "+
		"Make sure to place a default.RPP template file in your template directory for best results.",
		persistMessage, settings.ProjectDir, settings.TemplateDir, settings.DefaultTemplate), nil
}

// Settings interface implementation
func (m *musicProjectManagerTool) GetSettings() (string, error) {
	return globalSettings.GetSettings()
}

func (m *musicProjectManagerTool) SetSettings(settings string) error {
	return globalSettings.SetSettings(settings)
}

func (m *musicProjectManagerTool) GetDefaultSettings() (string, error) {
	return globalSettings.GetDefaultSettings()
}

func (m *musicProjectManagerTool) IsInitialized() bool {
	return globalSettings.IsInitialized()
}

// Version returns the plugin version.
func (m *musicProjectManagerTool) Version() string {
	return "1.0.0"
}

// SetAgentContext provides the current agent information to the plugin
func (m *musicProjectManagerTool) SetAgentContext(ctx AgentContext) {
	m.agentContext = &ctx
	globalAgentContext = &ctx
}

// SettingsManager implementation
func (sm *SettingsManager) GetSettings() (string, error) {
	if sm.settings == nil {
		sm.settings = sm.getDefaultSettings()
	}
	data, err := json.MarshalIndent(sm.settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal settings: %w", err)
	}
	return string(data), nil
}

func (sm *SettingsManager) SetSettings(settingsJSON string) error {
	var settings Settings
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}
	sm.settings = &settings
	return nil
}

func (sm *SettingsManager) GetDefaultSettings() (string, error) {
	defaults := sm.getDefaultSettings()
	data, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal default settings: %w", err)
	}
	return string(data), nil
}

func (sm *SettingsManager) IsInitialized() bool {
	if sm.settings == nil {
		sm.settings = sm.getDefaultSettings()
	}
	return sm.settings.Initialized
}

func (sm *SettingsManager) getDefaultSettings() *Settings {
	// Try to load defaults from agents.json first
	if defaults := loadDefaultsFromAgentsJSON(); defaults != nil {
		return defaults
	}

	// Fallback to hardcoded defaults
	return &Settings{
		DefaultTemplate: "/Users/jj/Music/Templates/default.RPP",
		ProjectDir:      "/Users/jj/Music/Projects",
		TemplateDir:     "/Users/jj/Music/Templates",
		Initialized:     false,
	}
}

// loadDefaultsFromAgentsJSON attempts to load default settings from individual agent files
func loadDefaultsFromAgentsJSON() *Settings {
	// Try to find any existing agent file with music_project_manager plugin
	possibleAgentPaths := []string{
		"./agents/reaper-project-manager.json",
		"./agents/default.json",
		"./agents/test.json",
		"./agents/reaper.json",
		"../agents/reaper-project-manager.json",
		"../agents/default.json",
		"../../agents/reaper-project-manager.json",
		"../../agents/default.json",
		"/Users/jj/Workspace/johnj-programming/projects/dolphin/dolphin-agent/agents/reaper-project-manager.json",
		"/Users/jj/Workspace/johnj-programming/projects/dolphin/dolphin-agent/agents/default.json",
	}

	for _, path := range possibleAgentPaths {
		if _, err := os.Stat(path); err == nil {
			// File exists, try to read it
			agentData, err := os.ReadFile(path)
			if err != nil {
				continue
			}

			var agentConfig IndividualAgentConfig
			if err := json.Unmarshal(agentData, &agentConfig); err != nil {
				continue
			}

			// Look for music_project_manager plugin in the agent config
			if plugin, exists := agentConfig.Plugins["music_project_manager"]; exists {
				if params, ok := plugin.Definition.Parameters["properties"].(map[string]interface{}); ok {
					settings := &Settings{
						Initialized: true, // If we found settings, consider initialized
					}

					// Extract default values from the plugin definition
					if projectDirParam, exists := params["project_dir"].(map[string]interface{}); exists {
						if defaultVal, hasDefault := projectDirParam["default"].(string); hasDefault {
							settings.ProjectDir = defaultVal
						}
					}

					if templateDirParam, exists := params["template_dir"].(map[string]interface{}); exists {
						if defaultVal, hasDefault := templateDirParam["default"].(string); hasDefault {
							settings.TemplateDir = defaultVal
							settings.DefaultTemplate = filepath.Join(defaultVal, "default.RPP")
						}
					}

					// If we got valid directories, return this settings object
					if settings.ProjectDir != "" && settings.TemplateDir != "" {
						return settings
					}
				}
			}
		}
	}

	return nil
}

func (sm *SettingsManager) getCurrentSettings() *Settings {
	if sm.settings == nil {
		sm.settings = sm.getDefaultSettings()
	}
	return sm.settings
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

// updateAgentSettings updates the agent's settings file with new directory settings
func (m *musicProjectManagerTool) updateAgentSettings(projectDir, templateDir string) error {
	// Check both instance and global agent context
	var agentContext *AgentContext
	if m.agentContext != nil && m.agentContext.SettingsPath != "" {
		agentContext = m.agentContext
	} else if globalAgentContext != nil && globalAgentContext.SettingsPath != "" {
		agentContext = globalAgentContext
	}

	if agentContext == nil {
		return fmt.Errorf("no agent context available - cannot determine settings file path")
	}

	settingsFilePath := agentContext.SettingsPath

	// Read existing settings or create default structure
	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		// Settings file doesn't exist, create default structure
		agentSettings = make(map[string]interface{})
	}

	// Ensure music_project_manager section exists
	if _, exists := agentSettings["music_project_manager"]; !exists {
		agentSettings["music_project_manager"] = make(map[string]interface{})
	}

	musicSettings := agentSettings["music_project_manager"].(map[string]interface{})

	// Update settings
	if projectDir != "" {
		musicSettings["project_dir"] = projectDir
		musicSettings["path"] = filepath.Dir(projectDir) // Also update parent directory
	}

	if templateDir != "" {
		musicSettings["template_dir"] = templateDir
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(filepath.Dir(settingsFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	// Write the updated settings
	updatedData, err := json.MarshalIndent(agentSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated agent settings: %w", err)
	}

	if err := os.WriteFile(settingsFilePath, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write to %s: %w", settingsFilePath, err)
	}

	return nil
}

// Tool is the exported symbol that the host application will look up.
var Tool = musicProjectManagerTool{}
