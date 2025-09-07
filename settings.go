package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/johnjallday/music_project_manager/common"
	"github.com/johnjallday/dolphin-agent/pluginapi"
)

// SettingsManager manages plugin settings
type SettingsManager struct {
	settings *common.Settings
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
	var settings common.Settings
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}
	sm.settings = &settings
	return nil
}

// UpdateSettings updates both in-memory settings and persists to agent_settings.json
func (sm *SettingsManager) UpdateSettings(projectDir, templateDir string, initialized bool, agentContext *pluginapi.AgentContext) error {
	if sm.settings == nil {
		sm.settings = sm.getDefaultSettings()
	}
	
	// Update in-memory settings
	if projectDir != "" {
		sm.settings.ProjectDir = projectDir
	}
	if templateDir != "" {
		sm.settings.TemplateDir = templateDir
		sm.settings.DefaultTemplate = filepath.Join(templateDir, "default.RPP")
	}
	sm.settings.Initialized = initialized
	
	// If we have agent context, also persist to agent_settings.json
	if agentContext != nil {
		return sm.persistToAgentSettings(projectDir, templateDir, agentContext)
	}
	
	return nil
}

// persistToAgentSettings saves the current settings to the agent's settings file
func (sm *SettingsManager) persistToAgentSettings(projectDir, templateDir string, agentContext *pluginapi.AgentContext) error {
	settingsFilePath := agentContext.SettingsPath

	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		agentSettings = make(map[string]interface{})
	}

	if _, exists := agentSettings["music_project_manager"]; !exists {
		agentSettings["music_project_manager"] = make(map[string]interface{})
	}

	musicSettings := agentSettings["music_project_manager"].(map[string]interface{})

	if projectDir != "" {
		musicSettings["project_dir"] = projectDir
		musicSettings["path"] = filepath.Dir(projectDir)
	}

	if templateDir != "" {
		musicSettings["template_dir"] = templateDir
		musicSettings["default_template"] = filepath.Join(templateDir, "default.RPP")
	}
	
	musicSettings["initialized"] = sm.settings.Initialized

	if err := os.MkdirAll(filepath.Dir(settingsFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	updatedData, err := json.MarshalIndent(agentSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal updated agent settings: %w", err)
	}

	if err := os.WriteFile(settingsFilePath, updatedData, 0644); err != nil {
		return fmt.Errorf("failed to write to %s: %w", settingsFilePath, err)
	}

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

func (sm *SettingsManager) getDefaultSettings() *common.Settings {
	// Try to load defaults from agents.json first
	if defaults := loadDefaultsFromAgentsJSON(); defaults != nil {
		return defaults
	}

	// Fallback to hardcoded defaults
	return &common.Settings{
		DefaultTemplate: "/Users/jj/Music/Templates/default.RPP",
		ProjectDir:      "/Users/jj/Music/Projects",
		TemplateDir:     "/Users/jj/Music/Templates",
		Initialized:     false,
	}
}

func (sm *SettingsManager) GetCurrentSettings() *common.Settings {
	if sm.settings == nil {
		sm.settings = sm.getDefaultSettings()
	}
	return sm.settings
}

// loadDefaultsFromAgentsJSON attempts to load default settings from individual agent files
func loadDefaultsFromAgentsJSON() *common.Settings {
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

			var agentConfig common.IndividualAgentConfig
			if err := json.Unmarshal(agentData, &agentConfig); err != nil {
				continue
			}

			// Look for music_project_manager plugin in the agent config
			if plugin, exists := agentConfig.Plugins["music_project_manager"]; exists {
				if params, ok := plugin.Definition.Parameters["properties"].(map[string]interface{}); ok {
					settings := &common.Settings{
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