package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/johnjallday/music_project_manager/common"
	"github.com/johnjallday/dolphin-agent/pluginapi"
)

// ProjectHandler handles music project operations
type ProjectHandler struct {
	agentContext *pluginapi.AgentContext
	settings     SettingsManager
}

// SettingsManager interface to avoid circular dependency
type SettingsManager interface {
	GetCurrentSettings() *common.Settings
	IsInitialized() bool
}

// NewProjectHandler creates a new project handler
func NewProjectHandler(agentContext *pluginapi.AgentContext, settings SettingsManager) *ProjectHandler {
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
		// If settings file doesn't exist, return empty/default settings instead of error
		formattedSettings := map[string]interface{}{
			"project_dir":      nil,
			"template_dir":     nil,
			"path":             nil,
			"initialized":      false,
			"default_template": nil,
			"status":           "Not configured - run setup to initialize",
		}

		data, err := json.MarshalIndent(formattedSettings, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal default settings: %w", err)
		}
		return string(data), nil
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
	// Expand tilde before setting
	expandedPath, err := expandTilde(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand home directory in path %q: %w", path, err)
	}

	settings := h.settings.GetCurrentSettings()
	settings.ProjectDir = expandedPath

	// Note: Persistence is handled by the main tool's SettingsManager.UpdateSettings()
	// We don't need to duplicate persistence here
	return fmt.Sprintf("✅ Project directory set to: %s", expandedPath), nil
}

// SetTemplateDir sets the template directory
func (h *ProjectHandler) SetTemplateDir(path string) (string, error) {
	// Expand tilde before setting
	expandedPath, err := expandTilde(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand home directory in path %q: %w", path, err)
	}

	if err := os.MkdirAll(expandedPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", expandedPath, err)
	}

	settings := h.settings.GetCurrentSettings()
	settings.TemplateDir = expandedPath
	if settings.DefaultTemplate != "" {
		settings.DefaultTemplate = filepath.Join(expandedPath, "default.RPP")
	}

	// Note: Persistence is handled by the main tool's SettingsManager.UpdateSettings()
	// We don't need to duplicate persistence here
	return fmt.Sprintf("✅ Template directory set to: %s", expandedPath), nil
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
	// Expand tilde in both paths
	expandedProjectDir, err := expandTilde(projectDir)
	if err != nil {
		return "", fmt.Errorf("failed to expand home directory in project path %q: %w", projectDir, err)
	}
	
	expandedTemplateDir, err := expandTilde(templateDir)
	if err != nil {
		return "", fmt.Errorf("failed to expand home directory in template path %q: %w", templateDir, err)
	}

	if err := os.MkdirAll(expandedProjectDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create project directory %s: %w", expandedProjectDir, err)
	}
	if err := os.MkdirAll(expandedTemplateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create template directory %s: %w", expandedTemplateDir, err)
	}

	settings := &common.Settings{
		ProjectDir:      expandedProjectDir,
		TemplateDir:     expandedTemplateDir,
		DefaultTemplate: filepath.Join(expandedTemplateDir, "default.RPP"),
		Initialized:     true,
	}

	// Note: Persistence is now handled by the main tool's SettingsManager.UpdateSettings()
	// This removes the duplicate persistence and agentContext dependency here

	return fmt.Sprintf("✅ Setup completed successfully!\n\n"+
		"Configuration saved:\n"+
		"- Project Directory: %s\n"+
		"- Template Directory: %s\n"+
		"- Default Template: %s\n\n"+
		"You can now use operation 'create_project' to create new music projects. "+
		"Make sure to place a default.RPP template file in your template directory for best results.",
		settings.ProjectDir, settings.TemplateDir, settings.DefaultTemplate), nil
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

	fmt.Println("Updating Agent")
	if h.agentContext == nil {
		fmt.Printf("WARNING: agentContext is nil - skipping agent settings persistence\n")
		// Don't return an error, just skip the persistence step
		// This allows the operation to continue with in-memory settings
		return nil
	}

	fmt.Printf("agentContext available: %+v\n", h.agentContext)
	settingsFilePath := h.agentContext.SettingsPath
	fmt.Println("settings file Path")
	fmt.Println(settingsFilePath)
	
	if settingsFilePath == "" {
		fmt.Printf("ERROR: settingsFilePath is empty\n")
		return fmt.Errorf("settings file path is empty")
	}

	var agentSettings map[string]interface{}
	if settingsData, err := os.ReadFile(settingsFilePath); err == nil {
		fmt.Printf("Successfully read settings file, size: %d bytes\n", len(settingsData))
		if err := json.Unmarshal(settingsData, &agentSettings); err != nil {
			fmt.Printf("ERROR: Failed to parse JSON: %v\n", err)
			return fmt.Errorf("failed to parse agent settings at %s: %w", settingsFilePath, err)
		}
		fmt.Printf("Successfully parsed JSON, keys: %v\n", getKeys(agentSettings))
	} else {
		fmt.Printf("Settings file doesn't exist or can't be read: %v, creating new\n", err)
		agentSettings = make(map[string]interface{})
	}

	if _, exists := agentSettings["music_project_manager"]; !exists {
		fmt.Println("Creating new music_project_manager section")
		agentSettings["music_project_manager"] = make(map[string]interface{})
	}

	musicSettings, ok := agentSettings["music_project_manager"].(map[string]interface{})
	if !ok {
		fmt.Printf("ERROR: music_project_manager is not a map: %T\n", agentSettings["music_project_manager"])
		return fmt.Errorf("invalid music_project_manager settings format")
	}

	fmt.Println("update works to here")

	if projectDir != "" {
		musicSettings["project_dir"] = projectDir
		musicSettings["path"] = filepath.Dir(projectDir)
	}

	if templateDir != "" {
		musicSettings["template_dir"] = templateDir
	}

	if err := os.MkdirAll(filepath.Dir(settingsFilePath), 0755); err != nil {
		fmt.Printf("ERROR: Failed to create directory %s: %v\n", filepath.Dir(settingsFilePath), err)
		return fmt.Errorf("failed to create agent directory: %w", err)
	}
	fmt.Printf("Successfully ensured directory exists: %s\n", filepath.Dir(settingsFilePath))

	updatedData, err := json.MarshalIndent(agentSettings, "", "  ")
	if err != nil {
		fmt.Printf("ERROR: Failed to marshal JSON: %v\n", err)
		return fmt.Errorf("failed to marshal updated agent settings: %w", err)
	}

	fmt.Println("update works to here")
	fmt.Printf("Final agentSettings: %+v\n", agentSettings)
	fmt.Printf("Writing to path: %s\n", settingsFilePath)
	fmt.Printf("Data to write (%d bytes): %s\n", len(updatedData), string(updatedData))

	if err := os.WriteFile(settingsFilePath, updatedData, 0644); err != nil {
		fmt.Printf("ERROR: Failed to write file: %v\n", err)
		return fmt.Errorf("failed to write to %s: %w", settingsFilePath, err)
	}

	fmt.Printf("SUCCESS: Settings successfully written to %s\n", settingsFilePath)
	return nil
}

// expandTilde expands ~ to the user's home directory
func expandTilde(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	
	if path == "~" {
		return usr.HomeDir, nil
	}
	
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(usr.HomeDir, path[2:]), nil
	}
	
	return path, nil
}

// getKeys is a helper function to get map keys for debugging
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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
