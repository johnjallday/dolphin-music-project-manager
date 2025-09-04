package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/johnjallday/music_project_manager/common"
)

// ProjectHandler handles music project operations
type ProjectHandler struct {
	agentContext *common.AgentContext
	settings     SettingsManager
}

// SettingsManager interface to avoid circular dependency
type SettingsManager interface {
	GetCurrentSettings() *common.Settings
	IsInitialized() bool
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(agentContext *common.AgentContext, settings SettingsManager) *ProjectHandler {
	return &ProjectHandler{
		agentContext: agentContext,
		settings:     settings,
	}
}

// CreateProject creates a new music project
func (h *ProjectHandler) CreateProject(name string, bpm int) (string, error) {
	agentSettings, err := h.getAgentSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load agent settings: %w", err)
	}

	if len(agentSettings) == 0 {
		return "Music Project Manager needs to be set up first. Please run music_project_manager with operation 'init_setup' to begin the setup process.", nil
	}

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

	defaultTemplate := filepath.Join(templateDir, "default.RPP")
	projectDir := filepath.Join(projectDirBase, name)
	
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory %q: %w", projectDir, err)
	}

	dest := filepath.Join(projectDir, name+".RPP")
	data, err := os.ReadFile(defaultTemplate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("template file not found at %q. Please ensure a default.RPP template exists in your template directory", defaultTemplate)
		}
		return "", fmt.Errorf("failed to read template file %q: %w", defaultTemplate, err)
	}
	
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write project file: %w", err)
	}

	if bpm > 0 {
		if err := updateProjectBPM(dest, bpm); err != nil {
			return "", fmt.Errorf("failed to update BPM in project file: %w", err)
		}
	}

	if err := launchReaper(dest); err != nil {
		return "", fmt.Errorf("failed to launch Reaper: %w", err)
	}

	msg := fmt.Sprintf("Created and launched project: %s", dest)
	if bpm > 0 {
		msg += fmt.Sprintf(" (BPM %d)", bpm)
	}
	return msg, nil
}

// GetSettings returns current settings from agent_settings.json
func (h *ProjectHandler) GetSettings() (string, error) {
	if h.agentContext == nil {
		settings := h.settings.GetCurrentSettings()
		data, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal settings: %w", err)
		}
		return string(data), nil
	}

	settingsFilePath := h.agentContext.SettingsPath

	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return "", fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		return "", fmt.Errorf("failed to read agent settings file at %s: %w", settingsFilePath, err)
	}

	var musicSettings map[string]interface{}
	if ms, exists := agentSettings["music_project_manager"].(map[string]interface{}); exists {
		musicSettings = ms
	} else {
		musicSettings = make(map[string]interface{})
	}

	formattedSettings := map[string]interface{}{
		"project_dir":  musicSettings["project_dir"],
		"template_dir": musicSettings["template_dir"],
		"path":         musicSettings["path"],
		"initialized":  len(musicSettings) > 0,
	}

	if templateDir, ok := musicSettings["template_dir"].(string); ok && templateDir != "" {
		formattedSettings["default_template"] = filepath.Join(templateDir, "default.RPP")
	}

	data, err := json.MarshalIndent(formattedSettings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal settings: %w", err)
	}
	return string(data), nil
}

// SetProjectDir sets the project directory
func (h *ProjectHandler) SetProjectDir(path string) (string, error) {
	settings := h.settings.GetCurrentSettings()
	settings.ProjectDir = path

	// Update agent settings to persist the setting
	if err := h.updateAgentSettings(path, ""); err != nil {
		return fmt.Sprintf("Project directory set to: %s\n⚠️  Could not persist to agent settings: %v\n\nPlease check that the agents directory is writable and the plugin has access to it.", path, err), nil
	}

	return fmt.Sprintf("✅ Project directory set to: %s\n✅ Successfully persisted to agent settings", path), nil
}

// SetTemplateDir sets the template directory
func (h *ProjectHandler) SetTemplateDir(path string) (string, error) {
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	settings := h.settings.GetCurrentSettings()
	settings.TemplateDir = path
	if settings.DefaultTemplate != "" {
		settings.DefaultTemplate = filepath.Join(path, "default.RPP")
	}

	if err := h.updateAgentSettings("", path); err != nil {
		return fmt.Sprintf("Template directory set to: %s\n⚠️  Could not persist to agent settings: %v\n\nPlease check that the agents directory is writable and the plugin has access to it.", path, err), nil
	}

	return fmt.Sprintf("✅ Template directory set to: %s\n✅ Successfully persisted to agent settings", path), nil
}

// InitSetup checks setup status and provides guidance
func (h *ProjectHandler) InitSetup() (string, error) {
	if h.settings.IsInitialized() {
		settings := h.settings.GetCurrentSettings()
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

// CompleteSetup completes initial setup
func (h *ProjectHandler) CompleteSetup(projectDir, templateDir string) (string, error) {
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory %s: %w", projectDir, err)
	}
	if err := os.MkdirAll(templateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create template directory %s: %w", templateDir, err)
	}

	settings := &common.Settings{
		ProjectDir:      projectDir,
		TemplateDir:     templateDir,
		DefaultTemplate: filepath.Join(templateDir, "default.RPP"),
		Initialized:     true,
	}

	// Note: We can't directly modify the settings through the interface
	// This would need to be handled differently in a real implementation
	
	persistMessage := ""
	if err := h.updateAgentSettings(projectDir, templateDir); err != nil {
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

// Helper functions

// updateProjectBPM updates the BPM in a project file
func updateProjectBPM(filePath string, bpm int) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
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
	
	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0644)
}

// launchReaper launches Reaper with the given project file
func launchReaper(projectPath string) error {
	cmd := exec.Command("open", "-a", "Reaper", projectPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getAgentSettings reads the music_project_manager settings from agent_settings.json
func (h *ProjectHandler) getAgentSettings() (map[string]interface{}, error) {
	if h.agentContext == nil {
		return nil, fmt.Errorf("no agent context available - cannot determine settings file path")
	}

	settingsFilePath := h.agentContext.SettingsPath

	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			return nil, fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
	} else {
		return nil, fmt.Errorf("failed to read agent settings file at %s: %w", settingsFilePath, err)
	}

	if musicSettings, exists := agentSettings["music_project_manager"].(map[string]interface{}); exists {
		return musicSettings, nil
	}

	return make(map[string]interface{}), nil
}

// updateAgentSettings updates the agent's settings file with new directory settings
func (h *ProjectHandler) updateAgentSettings(projectDir, templateDir string) error {
	if h.agentContext == nil {
		return fmt.Errorf("no agent context available - cannot determine settings file path")
	}

	settingsFilePath := h.agentContext.SettingsPath

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
	}

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