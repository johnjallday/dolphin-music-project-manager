package main

import (
	"bufio"
	"context"
	_ "embed"
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
	"github.com/johnjallday/ori-agent/pluginapi"
	"github.com/openai/openai-go/v2"
)

//go:embed plugin.yaml
var configYAML string

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
	config       pluginapi.PluginConfig
	settings     *Settings
	agentContext *pluginapi.AgentContext
}

// Version information set at build time via -ldflags
var (
	Version   = "dev"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

// Interface conformance checks
var (
	_ pluginapi.Tool                    = (*musicProjectManagerTool)(nil)
	_ pluginapi.VersionedTool           = (*musicProjectManagerTool)(nil)
	_ pluginapi.PluginCompatibility     = (*musicProjectManagerTool)(nil)
	_ pluginapi.MetadataProvider        = (*musicProjectManagerTool)(nil)
	_ pluginapi.DefaultSettingsProvider = (*musicProjectManagerTool)(nil)
	_ pluginapi.AgentAwareTool          = (*musicProjectManagerTool)(nil)
	_ pluginapi.InitializationProvider  = (*musicProjectManagerTool)(nil)
)

func (m *musicProjectManagerTool) Version() string {
	return m.config.Version
}

// MinAgentVersion returns the minimum ori-agent version required
func (m *musicProjectManagerTool) MinAgentVersion() string {
	return m.config.Requirements.MinOriVersion
}

// MaxAgentVersion returns the maximum compatible ori-agent version
func (m *musicProjectManagerTool) MaxAgentVersion() string {
	return "" // No maximum limit
}

// APIVersion returns the plugin API version
func (m *musicProjectManagerTool) APIVersion() string {
	return "v1"
}

// GetBuildInfo returns detailed build information
func (m *musicProjectManagerTool) GetBuildInfo() map[string]string {
	return map[string]string{
		"version":    Version,
		"build_time": BuildTime,
		"git_commit": GitCommit,
	}
}

// GetMetadata returns plugin metadata (maintainers, license, repository)
func (m *musicProjectManagerTool) GetMetadata() (*pluginapi.PluginMetadata, error) {
	return m.config.ToMetadata()
}

// Definition returns the OpenAI function definition for music project management operations.
func (m *musicProjectManagerTool) Definition() openai.FunctionDefinitionParam {
	return openai.FunctionDefinitionParam{
		Name:        "ori-music-project-manager",
		Description: openai.String("Manage Reaper DAW music projects (.RPP files). Use this for creating new music projects, opening existing Reaper projects in the DAW, opening project locations in Finder, scanning for project files, listing projects, filtering by BPM, and renaming projects. Examples: 'create project mash', 'open project beats', 'open beats in finder', 'show me my 140 BPM projects', 'rename China girl EDM to okok'"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{
					"type":        "string",
					"description": "Music project operation: create new Reaper project, open existing project in Reaper DAW, reveal project in Finder file browser, scan for .RPP files, list projects, filter by name/BPM, or rename an existing project",
					"enum":        []string{"create_project", "scan", "list_projects", "open_project", "open_in_finder", "filter_project", "rename_project"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Project name for creating new Reaper projects, filtering existing ones, finding projects to open in Finder, or the current name of a project to rename (e.g., 'mash', 'beats', 'Rich Daddy', 'China girl EDM')",
				},
				"new_name": map[string]any{
					"type":        "string",
					"description": "New name for the project when using rename_project operation (e.g., 'okok')",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Full file path to a Reaper project file (.RPP) to open in Reaper DAW or reveal in Finder (e.g., '/Users/name/Music/Projects/song.RPP')",
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
	var params struct {
		Operation string `json:"operation"`
		Name      string `json:"name"`
		NewName   string `json:"new_name"`
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
	case "scan":
		return m.scanProjects()
	case "list_projects":
		return m.listProjects()
	case "open_project":
		return m.openProject(params.Path)
	case "open_in_finder":
		return m.openInFinder(params.Path, params.Name)
	case "filter_project":
		return m.filterProject(params.Name, params.BPM, params.MinBPM, params.MaxBPM)
	case "rename_project":
		return m.renameProject(params.Name, params.NewName)
	default:
		return "", fmt.Errorf("unknown operation %q. Valid operations: create_project, scan, list_projects, open_project, open_in_finder, filter_project, rename_project", params.Operation)
	}
}

// createProject creates a new music project
func (m *musicProjectManagerTool) createProject(name string, bpm int) (string, error) {
	if err := validateCreateProject(name, bpm); err != nil {
		return "", err
	}

	settings, err := m.loadSettings()
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

	if err := os.MkdirAll(projectDir, 0o755); err != nil {
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

	if err := os.WriteFile(dest, data, 0o644); err != nil {
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

	// Append the new project to projects.json
	if err := m.appendProjectToJSON(dest, name, projectDirBase); err != nil {
		// Log the error but don't fail the operation since the project was created successfully
		log.Printf("[music-project-manager] Warning: failed to update projects.json: %v", err)
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

// openInFinder reveals a project file in Finder
func (m *musicProjectManagerTool) openInFinder(projectPath, projectName string) (string, error) {
	var targetPath string

	// If path is provided, use it directly
	if projectPath != "" {
		targetPath = projectPath
	} else if projectName != "" {
		// Search for project by name
		settings, err := m.loadSettings()
		if err != nil {
			return "", fmt.Errorf("failed to load settings: %w", err)
		}

		if settings.ProjectDir == "" {
			return "Music Project Manager needs to be configured. Please set project_dir in the application settings.", nil
		}

		// Look for projects.json file
		projectsFile := filepath.Join(settings.ProjectDir, "projects.json")
		data, err := os.ReadFile(projectsFile)
		if err != nil {
			return "", fmt.Errorf("projects.json not found at %s. Run 'scan' operation first", projectsFile)
		}

		// Parse the projects
		var projects []Project
		if err := json.Unmarshal(data, &projects); err != nil {
			return "", fmt.Errorf("failed to parse projects.json: %w", err)
		}

		// Search for matching project (case-insensitive, substring match)
		var matches []Project
		searchLower := strings.ToLower(projectName)
		for _, proj := range projects {
			if strings.Contains(strings.ToLower(proj.Name), searchLower) {
				matches = append(matches, proj)
			}
		}

		if len(matches) == 0 {
			return "", fmt.Errorf("no project found matching '%s'. Try running 'scan' to update the project list", projectName)
		}

		if len(matches) > 1 {
			// Return list of matches if ambiguous
			var matchNames []string
			for _, m := range matches {
				matchNames = append(matchNames, m.Name)
			}
			return "", fmt.Errorf("multiple projects found matching '%s': %s. Please be more specific", projectName, strings.Join(matchNames, ", "))
		}

		targetPath = matches[0].Path
	} else {
		return "", fmt.Errorf("either 'path' or 'name' must be provided")
	}

	// Check if the file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return "", fmt.Errorf("project file not found: %s", targetPath)
	}

	// Check if it's a .RPP file
	if strings.ToLower(filepath.Ext(targetPath)) != ".rpp" {
		return "", fmt.Errorf("file must be a .RPP (Reaper project) file, got: %s", filepath.Ext(targetPath))
	}

	// Open in Finder using -R flag to reveal the file
	cmd := exec.Command("open", "-R", targetPath)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to open in Finder: %w", err)
	}

	return fmt.Sprintf("Opened in Finder: %s", targetPath), nil
}

// scanProjects scans for .RPP files in the project directory and saves to projects.json
// Returns immediately and runs the scan in the background
func (m *musicProjectManagerTool) scanProjects() (string, error) {
	settings, err := m.loadSettings()
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

		err = os.WriteFile(projectsFile, projectsData, 0o644)
		if err != nil {
			log.Printf("[music-project-manager] Error writing projects.json: %v", err)
			return
		}

		log.Printf("[music-project-manager] Scan complete. Found %d projects and saved to %s", len(projects), projectsFile)
	}()

	return fmt.Sprintf("Scanning %s in the background. Use 'list_projects' to see results once complete.", projectDir), nil
}

// listProjects reads and returns the 30 most recent projects as a structured table result
func (m *musicProjectManagerTool) listProjects() (string, error) {
	settings, err := m.loadSettings()
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

	// Create simplified output with only name, path, and date
	type SimplifiedProject struct {
		Name string  `json:"name"`
		Path string  `json:"path"`
		Date string  `json:"date"`
		BPM  float64 `json:"bpm"`
	}

	simplified := make([]SimplifiedProject, len(recentProjects))
	for i, p := range recentProjects {
		simplified[i] = SimplifiedProject{
			Name: p.Name,
			Path: p.Path,
			Date: p.LastModified.Format("2006-01-02"),
			BPM:  p.BPM,
		}
	}

	// Create structured result for table display
	result := pluginapi.NewTableResult(
		"Recent Music Projects",
		[]string{"Name", "Path", "Date", "BPM"},
		simplified,
	)
	result.Description = fmt.Sprintf("Showing %d most recent projects", len(simplified))

	// Return as JSON
	return result.ToJSON()
}

// filterProject filters projects by name and/or BPM criteria
func (m *musicProjectManagerTool) filterProject(nameFilter string, exactBPM, minBPM, maxBPM int) (string, error) {
	settings, err := m.loadSettings()
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

	// Create simplified output with only name, path, and date
	type SimplifiedProject struct {
		Name string  `json:"name"`
		Path string  `json:"path"`
		Date string  `json:"date"`
		BPM  float64 `json:"bpm"`
	}

	simplified := make([]SimplifiedProject, len(recentProjects))
	for i, p := range recentProjects {
		simplified[i] = SimplifiedProject{
			Name: p.Name,
			Path: p.Path,
			Date: p.LastModified.Format("2006-01-02"),
			BPM:  p.BPM,
		}
	}

	// Create structured result for table display
	result := pluginapi.NewTableResult(
		"Filtered Music Projects",
		[]string{"Name", "Path", "Date", "BPM"},
		simplified,
	)
	result.Description = fmt.Sprintf("Found %d projects matching filters, showing %d most recent", len(filtered), limit)

	// Return as JSON
	return result.ToJSON()
}

// renameProject renames a project folder, its RPP file, and updates projects.json
func (m *musicProjectManagerTool) renameProject(oldName, newName string) (string, error) {
	// Validate inputs
	if oldName == "" {
		return "", fmt.Errorf("old project name is required")
	}
	if newName == "" {
		return "", fmt.Errorf("new project name is required")
	}

	// Validate new name doesn't contain invalid characters
	if strings.ContainsAny(newName, `<>:"/\|?*`) {
		return "", fmt.Errorf("new project name contains invalid characters. Avoid: < > : \" / \\ | ? *")
	}

	// Load settings
	settings, err := m.loadSettings()
	if err != nil {
		return "", fmt.Errorf("failed to load settings: %w", err)
	}

	if settings.ProjectDir == "" {
		return "Music Project Manager needs to be configured. Please set project_dir in the application settings.", nil
	}

	projectDir := settings.ProjectDir
	projectsFile := filepath.Join(projectDir, "projects.json")

	// Read projects.json
	data, err := os.ReadFile(projectsFile)
	if err != nil {
		return "", fmt.Errorf("projects.json not found at %s. Run 'scan' operation first", projectsFile)
	}

	// Parse projects
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return "", fmt.Errorf("failed to parse projects.json: %w", err)
	}

	// Find the project to rename
	var projectToRename *Project
	var projectIndex int
	searchLower := strings.ToLower(oldName)

	for i, proj := range projects {
		if strings.EqualFold(proj.Name, oldName) || strings.Contains(strings.ToLower(proj.Name), searchLower) {
			projectToRename = &projects[i]
			projectIndex = i
			break
		}
	}

	if projectToRename == nil {
		return "", fmt.Errorf("project '%s' not found. Try running 'scan' to update the project list", oldName)
	}

	// Get old and new paths
	oldProjectPath := projectToRename.Path
	oldFolderPath := filepath.Dir(oldProjectPath)
	oldRPPName := filepath.Base(oldProjectPath)

	// Construct new paths
	newFolderPath := filepath.Join(filepath.Dir(oldFolderPath), newName)
	newRPPPath := filepath.Join(newFolderPath, newName+".RPP")

	// Check if target folder already exists
	if _, err := os.Stat(newFolderPath); err == nil {
		return "", fmt.Errorf("a project folder named '%s' already exists", newName)
	}

	// Step 1: Rename the folder
	if err := os.Rename(oldFolderPath, newFolderPath); err != nil {
		return "", fmt.Errorf("failed to rename project folder from '%s' to '%s': %w", oldFolderPath, newFolderPath, err)
	}

	// Step 2: Rename the RPP file inside the renamed folder
	tempOldRPPPath := filepath.Join(newFolderPath, oldRPPName)
	if err := os.Rename(tempOldRPPPath, newRPPPath); err != nil {
		// Try to rollback folder rename
		os.Rename(newFolderPath, oldFolderPath)
		return "", fmt.Errorf("failed to rename RPP file: %w", err)
	}

	// Step 3: Update projects.json
	projects[projectIndex].Name = newName
	projects[projectIndex].Path = newRPPPath

	// Get new file info
	if fileInfo, err := os.Stat(newRPPPath); err == nil {
		projects[projectIndex].LastModified = fileInfo.ModTime()
		projects[projectIndex].Size = fileInfo.Size()
	}

	// Write updated projects.json
	projectsData, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal updated projects data: %w", err)
	}

	if err := os.WriteFile(projectsFile, projectsData, 0o644); err != nil {
		return "", fmt.Errorf("failed to write updated projects.json: %w", err)
	}

	log.Printf("[music-project-manager] Successfully renamed project from '%s' to '%s'", oldName, newName)
	return fmt.Sprintf("Successfully renamed project from '%s' to '%s'\nOld path: %s\nNew path: %s", oldName, newName, oldProjectPath, newRPPPath), nil
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

// SetAgentContext provides the current agent information to the plugin
func (m *musicProjectManagerTool) SetAgentContext(ctx pluginapi.AgentContext) {
	m.agentContext = &ctx
}

// InitializationProvider implementation for modern plugin configuration

// GetRequiredConfig returns the configuration variables needed by this plugin
func (m *musicProjectManagerTool) GetRequiredConfig() []pluginapi.ConfigVariable {
	usr, _ := user.Current()
	defaultProjectDir := filepath.Join(usr.HomeDir, "Music", "Projects")
	defaultTemplateDir := filepath.Join(usr.HomeDir, "Library", "Application Support", "REAPER", "ProjectTemplates")
	defaultTemplatePath := filepath.Join(defaultTemplateDir, "Default.RPP")

	return []pluginapi.ConfigVariable{
		{
			Key:          "project_dir",
			Name:         "Project Directory",
			Description:  "Directory where music projects are stored",
			Type:         pluginapi.ConfigTypeDirPath,
			Required:     true,
			DefaultValue: defaultProjectDir,
			Placeholder:  defaultProjectDir,
		},
		{
			Key:          "template_dir",
			Name:         "Template Directory",
			Description:  "Directory where REAPER project templates are stored",
			Type:         pluginapi.ConfigTypeDirPath,
			Required:     true,
			DefaultValue: defaultTemplateDir,
			Placeholder:  defaultTemplateDir,
		},
		{
			Key:          "default_template",
			Name:         "Default Template",
			Description:  "Path to the default REAPER project template file (.RPP)",
			Type:         pluginapi.ConfigTypeFilePath,
			Required:     false,
			DefaultValue: defaultTemplatePath,
			Placeholder:  defaultTemplatePath,
		},
	}
}

// ValidateConfig validates the provided configuration
func (m *musicProjectManagerTool) ValidateConfig(config map[string]interface{}) error {
	projectDir, ok := config["project_dir"].(string)
	if !ok || projectDir == "" {
		return fmt.Errorf("project_dir is required")
	}

	templateDir, ok := config["template_dir"].(string)
	if !ok || templateDir == "" {
		return fmt.Errorf("template_dir is required")
	}

	// default_template is optional, but if provided, validate it
	if defaultTemplate, ok := config["default_template"].(string); ok && defaultTemplate != "" {
		if !strings.HasSuffix(strings.ToLower(defaultTemplate), ".rpp") {
			return fmt.Errorf("default_template must be a .RPP file")
		}
	}

	return nil
}

// InitializeWithConfig initializes the plugin with the provided configuration
func (m *musicProjectManagerTool) InitializeWithConfig(config map[string]interface{}) error {
	projectDir, _ := config["project_dir"].(string)
	templateDir, _ := config["template_dir"].(string)
	defaultTemplate, _ := config["default_template"].(string)

	// If default_template is not provided, construct it from template_dir
	if defaultTemplate == "" {
		defaultTemplate = filepath.Join(templateDir, "default.RPP")
	}

	// Create Settings struct from config
	newSettings := &Settings{
		ProjectDir:      projectDir,
		TemplateDir:     templateDir,
		DefaultTemplate: defaultTemplate,
	}

	// Update in-memory settings
	m.settings = newSettings

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

// loadSettings loads settings from memory or file
func (m *musicProjectManagerTool) loadSettings() (*Settings, error) {
	// Check if settings are already loaded in memory
	if m.settings != nil {
		return m.settings, nil
	}

	// Load from file
	return m.loadSettingsFromFile()
}

// loadSettingsFromFile loads settings from agent-specific settings file
func (m *musicProjectManagerTool) loadSettingsFromFile() (*Settings, error) {
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

// appendProjectToJSON appends a newly created project to the projects.json file
func (m *musicProjectManagerTool) appendProjectToJSON(projectPath, projectName, projectDirBase string) error {
	projectsFile := filepath.Join(projectDirBase, "projects.json")

	// Get file info for the new project
	fileInfo, err := os.Stat(projectPath)
	if err != nil {
		return fmt.Errorf("failed to stat project file: %w", err)
	}

	// Extract BPM from the project file
	bpm, err := extractBPMFromRPP(projectPath)
	if err != nil {
		log.Printf("[music-project-manager] Warning: failed to extract BPM from %s: %v", projectPath, err)
		bpm = 0 // Use 0 as default if extraction fails
	}

	// Create the new project entry
	newProject := Project{
		Name:         projectName,
		Path:         projectPath,
		LastModified: fileInfo.ModTime(),
		Size:         fileInfo.Size(),
		BPM:          bpm,
	}

	// Read existing projects.json if it exists
	var projects []Project
	if data, err := os.ReadFile(projectsFile); err == nil {
		// File exists, parse it
		if err := json.Unmarshal(data, &projects); err != nil {
			return fmt.Errorf("failed to parse existing projects.json: %w", err)
		}
	}
	// If file doesn't exist or is empty, projects will be an empty slice, which is fine

	// Append the new project
	projects = append(projects, newProject)

	// Write back to projects.json
	projectsData, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal projects data: %w", err)
	}

	if err := os.WriteFile(projectsFile, projectsData, 0o644); err != nil {
		return fmt.Errorf("failed to write projects.json: %w", err)
	}

	log.Printf("[music-project-manager] Successfully appended project '%s' to projects.json", projectName)
	return nil
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

	return os.WriteFile(filePath, []byte(strings.Join(lines, "\n")), 0o644)
}

// launchReaper launches Reaper with the given project file
func launchReaper(projectPath string) error {
	cmd := exec.Command("open", "-a", "Reaper", projectPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	// Parse plugin config from embedded YAML
	config := pluginapi.ReadPluginConfig(configYAML)

	plugin.Serve(&plugin.ServeConfig{
		HandshakeConfig: pluginapi.Handshake,
		Plugins: map[string]plugin.Plugin{
			"tool": &pluginapi.ToolRPCPlugin{Impl: &musicProjectManagerTool{config: config}},
		},
		GRPCServer: plugin.DefaultGRPCServer,
	})
}
