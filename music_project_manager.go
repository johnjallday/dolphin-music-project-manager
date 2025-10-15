package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/johnjallday/dolphin-agent/pluginapi"
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
	BPM          float64   `json:"bpm"`
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
		Description: openai.String("Manage music projects: create projects, open existing projects, scan for .RPP files, list existing projects, filter recent projects, and view settings"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "Operation to perform",
					"enum":        []string{"create_project", "get_settings", "scan", "list_projects", "open_project", "filter_project"},
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
					"description": "BPM for the project (optional for create_project, exact BPM for filter_project)",
					"minimum":     30,
					"maximum":     300,
				},
				"min_bpm": map[string]any{
					"type":        "integer",
					"description": "Minimum BPM for filter_project (optional)",
					"minimum":     30,
					"maximum":     300,
				},
				"max_bpm": map[string]any{
					"type":        "integer",
					"description": "Maximum BPM for filter_project (optional)",
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
		MinBPM    int    `json:"min_bpm"`
		MaxBPM    int    `json:"max_bpm"`
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
	case "filter_project":
		return m.filterProject(params.Name, params.BPM, params.MinBPM, params.MaxBPM)
	default:
		return "", fmt.Errorf("unknown operation %q. Valid operations: create_project, get_settings, scan, list_projects, open_project, filter_project", params.Operation)
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
// Returns immediately and runs the scan in the background
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

	// Start scanning in the background
	go func() {
		log.Printf("[music-project-manager] Starting background scan of %s", projectDir)

		var projects []Project

		err := filepath.Walk(projectDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			// Check if file has .RPP extension (Reaper project files)
			if strings.ToLower(filepath.Ext(path)) == ".rpp" {
				// Extract BPM from the RPP file
				bpm, err := extractBPMFromRPP(path)
				if err != nil {
					log.Printf("[music-project-manager] Warning: failed to extract BPM from %s: %v", path, err)
					bpm = 0 // Use 0 as default if extraction fails
				}

				project := Project{
					Name:         strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)),
					Path:         path,
					LastModified: info.ModTime(),
					Size:         info.Size(),
					BPM:          bpm,
				}
				projects = append(projects, project)
			}
			return nil
		})

		if err != nil {
			log.Printf("[music-project-manager] Error scanning directory: %v", err)
			return
		}

		// Create projects.json file in the project directory
		projectsFile := filepath.Join(projectDir, "projects.json")

		projectsData, err := json.MarshalIndent(projects, "", "  ")
		if err != nil {
			log.Printf("[music-project-manager] Error marshaling projects data: %v", err)
			return
		}

		err = os.WriteFile(projectsFile, projectsData, 0644)
		if err != nil {
			log.Printf("[music-project-manager] Error writing projects.json: %v", err)
			return
		}

		log.Printf("[music-project-manager] Scan complete. Found %d projects and saved to %s", len(projects), projectsFile)
	}()

	return fmt.Sprintf("Scanning %s in the background. Use 'list_projects' to see results once complete.", projectDir), nil
}

// listProjects reads and returns the 30 most recent projects as JSON
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

	// Parse the JSON
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return "", fmt.Errorf("failed to parse projects.json: %w", err)
	}

	if len(projects) == 0 {
		return fmt.Sprintf("No projects found in %s", projectsFile), nil
	}

	// Sort projects by LastModified time in descending order (most recent first)
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].LastModified.After(projects[j].LastModified)
	})

	// Take only the first 30 projects (or fewer if less than 30 exist)
	limit := 30
	if len(projects) < limit {
		limit = len(projects)
	}
	recentProjects := projects[:limit]

	// Return JSON array for frontend table rendering
	projectsData, err := json.MarshalIndent(recentProjects, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling projects data: %w", err)
	}

	return string(projectsData), nil
}

// filterProject filters projects by name and/or BPM criteria
func (m *musicProjectManagerTool) filterProject(nameFilter string, exactBPM, minBPM, maxBPM int) (string, error) {
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

	// Parse the JSON
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return "", fmt.Errorf("failed to parse projects.json: %w", err)
	}

	if len(projects) == 0 {
		return fmt.Sprintf("No projects found in %s", projectsFile), nil
	}

	// Filter projects based on criteria
	var filtered []Project
	for _, proj := range projects {
		// Filter by name (case-insensitive substring match)
		if nameFilter != "" && !strings.Contains(strings.ToLower(proj.Name), strings.ToLower(nameFilter)) {
			continue
		}

		// Filter by exact BPM if specified
		if exactBPM > 0 && int(proj.BPM) != exactBPM {
			continue
		}

		// Filter by BPM range if specified
		if minBPM > 0 && proj.BPM < float64(minBPM) {
			continue
		}
		if maxBPM > 0 && proj.BPM > float64(maxBPM) {
			continue
		}

		filtered = append(filtered, proj)
	}

	if len(filtered) == 0 {
		return "No projects match the filter criteria", nil
	}

	// Sort filtered projects by LastModified time in descending order (most recent first)
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].LastModified.After(filtered[j].LastModified)
	})

	// Take only the first 30 projects (or fewer if less than 30 exist)
	limit := 30
	if len(filtered) < limit {
		limit = len(filtered)
	}
	recentProjects := filtered[:limit]

	// Convert to JSON
	projectsData, err := json.MarshalIndent(recentProjects, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling filtered projects data: %w", err)
	}

	return fmt.Sprintf("Found %d projects matching filters, showing %d most recent:\n%s", len(filtered), limit, string(projectsData)), nil
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

// GetSettings returns current settings as JSON (implementing settings interface)
func (m *musicProjectManagerTool) GetSettings() (string, error) {
	settings, err := m.getSettings()
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal current settings: %w", err)
	}
	return string(data), nil
}

// SetSettings updates the plugin settings from JSON (implementing settings interface)
func (m *musicProjectManagerTool) SetSettings(settingsJSON string) error {
	var newSettings Settings
	if err := json.Unmarshal([]byte(settingsJSON), &newSettings); err != nil {
		return fmt.Errorf("failed to unmarshal settings: %w", err)
	}

	// Get current agent from agents.json file
	currentAgent, err := m.getCurrentAgentFromFile()
	if err != nil {
		return fmt.Errorf("failed to get current agent: %w", err)
	}

	// Ensure agents directory exists
	agentDir := filepath.Join(".", "agents", currentAgent)
	if err := os.MkdirAll(agentDir, 0755); err != nil {
		return fmt.Errorf("failed to create agent directory: %w", err)
	}

	// Save settings to agent-specific file
	settingsPath := filepath.Join(agentDir, "music-project-manager_settings.json")
	data, err := json.MarshalIndent(newSettings, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal settings for saving: %w", err)
	}

	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write settings file: %w", err)
	}

	// Reset getSettings so it reloads from file on next call
	// This ensures settings changes are picked up immediately
	m.getSettings = nil

	return nil
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
		log.Printf("[music-project-manager] Failed to get current agent: %v, using defaults", err)
		// Fall back to default settings if no agent file or error reading it
		return m.getDefaultSettings()
	}

	// Try to load settings from the agent-specific file
	settingsPath := filepath.Join(".", "agents", currentAgent, "music-project-manager_settings.json")
	log.Printf("[music-project-manager] Attempting to load settings from: %s", settingsPath)

	data, err := os.ReadFile(settingsPath)
	if err != nil {
		log.Printf("[music-project-manager] Failed to read settings file: %v, using defaults", err)
		return m.getDefaultSettings()
	}

	var settings Settings
	if err := json.Unmarshal(data, &settings); err != nil {
		log.Printf("[music-project-manager] Failed to unmarshal settings: %v, using defaults", err)
		return m.getDefaultSettings()
	}

	log.Printf("[music-project-manager] Successfully loaded settings: project_dir=%s", settings.ProjectDir)
	return &settings, nil
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
// extractBPMFromRPP reads an RPP file and extracts the BPM value from the TEMPO line
// Only reads the first 100 lines for performance (TEMPO is typically near the top)
func extractBPMFromRPP(filePath string) (float64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	maxLines := 100 // Only scan first 100 lines for performance

	for scanner.Scan() && lineCount < maxLines {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")

		// Look for "TEMPO " with a space after it
		if strings.HasPrefix(trimmed, "TEMPO ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				// Parse the BPM value (second field after "TEMPO")
				bpm, err := strconv.ParseFloat(parts[1], 64)
				if err != nil {
					return 0, fmt.Errorf("failed to parse BPM value: %w", err)
				}
				return bpm, nil
			}
		}
		lineCount++
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	// If no TEMPO line found in first 100 lines, return 0
	return 0, nil
}

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

func main() {
	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.Handshake,
		Plugins: map[string]plugin.Plugin{
			"tool": &pluginapi.ToolRPCPlugin{Impl: &musicProjectManagerTool{}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
