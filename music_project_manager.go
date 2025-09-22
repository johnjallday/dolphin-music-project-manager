package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go/v2"
)

// PluginTool is the interface that plugins must implement to be used as tools.
type PluginTool interface {
	// Definition returns the function definition for OpenAI function calling.
	Definition() openai.FunctionDefinitionParam
	// Call executes the tool logic with the given arguments JSON string and returns the result JSON string.
	Call(ctx context.Context, args string) (string, error)
}

// Settings represents the plugin configuration
type Settings struct {
	DefaultTemplate string `json:"default_template"`
	ProjectDir      string `json:"project_dir"`
	TemplateDir     string `json:"template_dir"`
}

// Project represents a music project
type Project struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	LastModified time.Time `json:"lastModified"`
	Size         int64     `json:"size"`
}

// AgentsConfig represents the agents.json file structure
type AgentsConfig struct {
	CurrentAgent string `json:"current"`
}

// musicProjectManagerTool implements Tool for music project management.
type musicProjectManagerTool struct {
	getSettings func() (*Settings, error)
}

// Version information set at build time via -ldflags
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func (m *musicProjectManagerTool) Version() string {
	return Version
}

// GetBuildInfo returns detailed build information
func (m *musicProjectManagerTool) GetBuildInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"build_time": BuildTime,
		"git_commit": GitCommit,
	}
}

// Definition returns the OpenAI function definition for music project management operations.
func (m *musicProjectManagerTool) Definition() openai.FunctionDefinitionParam {
	return openai.FunctionDefinitionParam{
		Name:        "music_project_manager",
		Description: openai.String("Manage music projects: create projects, open existing projects, scan for .RPP files, list existing projects, and view settings"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "Operation to perform",
					"enum":        []string{"create_project", "get_settings", "scan", "list_projects", "open_project"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Project name (required for create_project)",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Project file path (required for open_project)",
				},
				"bpm": map[string]any{
					"type":        "integer",
					"description": "BPM for the project (optional for create_project)",
					"minimum":     30,
					"maximum":     300,
				},
			},
			"required": []string{"operation"},
		},
	}
}

// Call is invoked with the function arguments and dispatches to the appropriate operation.
func (m *musicProjectManagerTool) Call(ctx context.Context, args string) (string, error) {
	if m.getSettings == nil {
		m.getSettings = m.loadSettingsFromAPI
	}

	var params struct {
		Operation string `json:"operation"`
		Name      string `json:"name"`
		Path      string `json:"path"`
		BPM       int    `json:"bpm"`
	}

	if err := json.Unmarshal([]byte(args), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	switch params.Operation {
	case "create_project":
		return m.createProject(params.Name, params.BPM)
	case "get_settings":
		return m.getSettingsStruct()
	case "scan":
		return m.scanProjects()
	case "list_projects":
		return m.listProjects()
	case "open_project":
		return m.openProject(params.Path)
	default:
		return "", fmt.Errorf("unknown operation %q. Valid operations: create_project, get_settings, scan, list_projects, open_project", params.Operation)
	}
}

// createProject creates a new music project
func (m *musicProjectManagerTool) createProject(name string, bpm int) (string, error) {
	if err := validateCreateProject(name, bpm); err != nil {
		return "", err
	}

	settings, err := m.getSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load settings: %w", err)
	}

	if settings.ProjectDir == "" || settings.TemplateDir == "" {
		return "Music Project Manager needs to be configured. Please set project_dir and template_dir in the application settings.", nil
	}

	projectDirBase := settings.ProjectDir
	templateDir := settings.TemplateDir

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

// openProject opens an existing project using launchReaper
func (m *musicProjectManagerTool) openProject(projectPath string) (string, error) {
	if projectPath == "" {
		return "", fmt.Errorf("project path is required and cannot be empty")
	}

	// Check if the file exists
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return "", fmt.Errorf("project file not found: %s", projectPath)
	}

	// Check if it's a .RPP file
	if strings.ToLower(filepath.Ext(projectPath)) != ".rpp" {
		return "", fmt.Errorf("file must be a .RPP (Reaper project) file, got: %s", filepath.Ext(projectPath))
	}

	// Launch Reaper with the project file
	if err := launchReaper(projectPath); err != nil {
		return "", fmt.Errorf("failed to launch Reaper with project %s: %w", projectPath, err)
	}

	return fmt.Sprintf("Opened project: %s", projectPath), nil
}

// scanProjects scans for .RPP files in the project directory and saves to projects.json
func (m *musicProjectManagerTool) scanProjects() (string, error) {
	settings, err := m.getSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load settings: %w", err)
	}

	if settings.ProjectDir == "" {
		return "Music Project Manager needs to be configured. Please set project_dir in the application settings.", nil
	}

	projectDir := settings.ProjectDir

	// Check if project directory exists
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		return fmt.Sprintf("Project directory does not exist: %s", projectDir), nil
	}

	var projects []Project

	err = filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if file has .RPP extension (Reaper project files)
		if strings.ToLower(filepath.Ext(path)) == ".rpp" {
			project := Project{
				Name:         strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
				Path:         path,
				LastModified: info.ModTime(),
				Size:         info.Size(),
			}
			projects = append(projects, project)
		}
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("error scanning directory: %w", err)
	}

	// Create projects.json file in the project directory
	projectsFile := filepath.Join(projectDir, "projects.json")

	projectsData, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling projects data: %w", err)
	}

	err = os.WriteFile(projectsFile, projectsData, 0644)
	if err != nil {
		return "", fmt.Errorf("error writing projects.json: %w", err)
	}

	return fmt.Sprintf("Found %d .RPP projects and saved to %s:\n%s", len(projects), projectsFile, string(projectsData)), nil
}

// listProjects reads and returns the content of projects.json
func (m *musicProjectManagerTool) listProjects() (string, error) {
	settings, err := m.getSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load settings: %w", err)
	}

	if settings.ProjectDir == "" {
		return "Music Project Manager needs to be configured. Please set project_dir in the application settings.", nil
	}

	projectDir := settings.ProjectDir
	projectsFile := filepath.Join(projectDir, "projects.json")

	// Check if projects.json exists
	if _, err := os.Stat(projectsFile); os.IsNotExist(err) {
		return fmt.Sprintf("No projects.json file found at %s. Run 'scan' operation first to generate the projects list.", projectsFile), nil
	}

	// Read the projects.json file
	data, err := os.ReadFile(projectsFile)
	if err != nil {
		return "", fmt.Errorf("failed to read projects.json: %w", err)
	}

	// Parse the JSON to validate it and get project count
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return "", fmt.Errorf("failed to parse projects.json: %w", err)
	}

	// Return formatted output with project count and the JSON content
	return fmt.Sprintf("Found %d projects in %s:\n%s", len(projects), projectsFile, string(data)), nil
}

// getSettingsStruct returns the field names and types of the Settings struct
func (m *musicProjectManagerTool) getSettingsStruct() (string, error) {
	fieldInfo := map[string]string{
		"default_template": "filepath",
		"project_dir":      "filepath",
		"template_dir":     "filepath",
	}
	data, err := json.Marshal(fieldInfo)
	if err != nil {
		return "", fmt.Errorf("failed to marshal field info: %w", err)
	}
	return string(data), nil
}

// GetDefaultSettings returns default settings as JSON (implementing pluginapi interface)
func (m *musicProjectManagerTool) GetDefaultSettings() (string, error) {
	defaultSettings, err := m.getDefaultSettings()
	if err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(defaultSettings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal default settings: %w", err)
	}

	return string(data), nil
}

// validateCreateProject validates parameters for create_project operation
func validateCreateProject(name string, bpm int) error {
	if name == "" {
		return fmt.Errorf("project name is required and cannot be empty")
	}

	// Validate project name doesn't contain invalid characters
	if strings.ContainsAny(name, `<>:"/\|?*`) {
		return fmt.Errorf("project name contains invalid characters. Avoid: < > : \" / \\ | ? *")
	}

	// Validate BPM range if provided
	if bpm != 0 && (bpm < 30 || bpm > 300) {
		return fmt.Errorf("BPM must be between 30 and 300, got %d", bpm)
	}

	return nil
}

// loadSettingsFromAPI loads settings from agent-specific settings file
func (m *musicProjectManagerTool) loadSettingsFromAPI() (*Settings, error) {
	// Get current agent from agents.json file
	currentAgent, err := m.getCurrentAgentFromFile()
	if err != nil {
		// Fall back to default settings if no agent file or error reading it
		return m.getDefaultSettings()
	}

	// Try to load settings from the agent-specific file
	settingsPath := filepath.Join(".", "agents", currentAgent, "music_project_manager_settings.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var settings Settings
		if err := json.Unmarshal(data, &settings); err == nil {
			return &settings, nil
		}
	}

	// Fall back to default settings if file doesn't exist or is invalid
	return m.getDefaultSettings()
}

// getCurrentAgentFromFile reads the current agent from agents.json
func (m *musicProjectManagerTool) getCurrentAgentFromFile() (string, error) {
	agentsFilePath := filepath.Join(".", "agents.json")
	data, err := os.ReadFile(agentsFilePath)
	if err != nil {
		return "", fmt.Errorf("failed to read agents.json: %w", err)
	}

	var agentsConfig AgentsConfig
	if err := json.Unmarshal(data, &agentsConfig); err != nil {
		return "", fmt.Errorf("failed to parse agents.json: %w", err)
	}

	if agentsConfig.CurrentAgent == "" {
		return "", fmt.Errorf("no current agent set in agents.json")
	}

	return agentsConfig.CurrentAgent, nil
}

// getDefaultSettings returns default settings for the music project manager
func (m *musicProjectManagerTool) getDefaultSettings() (*Settings, error) {
	usr, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	return &Settings{
		ProjectDir:      filepath.Join(usr.HomeDir, "Music", "Projects"),
		TemplateDir:     filepath.Join(usr.HomeDir, "Library", "Application Support", "REAPER", "ProjectTemplates"),
		DefaultTemplate: filepath.Join(usr.HomeDir, "Library", "Application Support", "REAPER", "ProjectTemplates", "Default.RPP"),
	}, nil
}

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

// Tool is the exported symbol that the host application will look up.
var Tool = musicProjectManagerTool{}
