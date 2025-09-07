package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/johnjallday/dolphin-agent/pluginapi"
	"github.com/johnjallday/music_project_manager/handlers"
	"github.com/openai/openai-go/v2"
)

// musicProjectManagerTool implements Tool for music project management.
type musicProjectManagerTool struct {
	agentContext   *pluginapi.AgentContext
	projectHandler *handlers.ProjectHandler
	settings       *SettingsManager
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

func (m *musicProjectManagerTool) initializeHandler() {
	if m.projectHandler == nil {
		if m.settings == nil {
			m.settings = &SettingsManager{}
		}
		m.projectHandler = handlers.NewProjectHandler(m.agentContext, m.settings)
	}
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
					"minimum":     30,
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
	if m.projectHandler == nil {
		m.initializeHandler()
	}

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

	// Allow certain operations even when not initialized
	allowedWhenUninitialized := []string{"get_settings", "init_setup", "complete_setup"}
	operationAllowed := false
	for _, allowed := range allowedWhenUninitialized {
		if p.Operation == allowed {
			operationAllowed = true
			break
		}
	}

	// Auto-initialize if not initialized and operation requires it
	if !m.IsInitialized() && !operationAllowed {
		// Return initialization prompt instead of proceeding with operation
		return m.projectHandler.InitSetup()
	}

	switch p.Operation {
	case "create_project":
		if err := validateCreateProject(p.Name, p.BPM); err != nil {
			return "", err
		}
		return m.projectHandler.CreateProject(p.Name, p.BPM)

	case "set_project_dir":
		if err := validatePath(p.Path, "project directory"); err != nil {
			return "", err
		}
		result, err := m.projectHandler.SetProjectDir(p.Path)
		if err != nil {
			return "", err
		}

		// Expand tilde before persisting
		expandedPath, err := expandTilde(p.Path)
		if err != nil {
			fmt.Printf("Warning: Failed to expand project directory path: %v\n", err)
			expandedPath = p.Path
		}

		// Update in-memory settings only (persistence handled by ProjectHandler)
		if m.settings != nil {
			if updateErr := m.settings.UpdateSettings(expandedPath, "", m.IsInitialized(), nil); updateErr != nil {
				// Log the error but don't fail the operation
				fmt.Printf("Warning: Failed to update in-memory project directory setting: %v\n", updateErr)
			}
		}

		return result, nil

	case "set_template_dir":
		if err := validatePath(p.Path, "template directory"); err != nil {
			return "", err
		}
		result, err := m.projectHandler.SetTemplateDir(p.Path)
		if err != nil {
			return "", err
		}

		// Expand tilde before persisting
		expandedPath, err := expandTilde(p.Path)
		if err != nil {
			fmt.Printf("Warning: Failed to expand template directory path: %v\n", err)
			expandedPath = p.Path
		}

		// Update in-memory settings only (persistence handled by ProjectHandler)
		if m.settings != nil {
			if updateErr := m.settings.UpdateSettings("", expandedPath, m.IsInitialized(), nil); updateErr != nil {
				// Log the error but don't fail the operation
				fmt.Printf("Warning: Failed to update in-memory template directory setting: %v\n", updateErr)
			}
		}

		return result, nil

	case "get_settings":
		return m.projectHandler.GetSettings()

	case "init_setup":
		return m.projectHandler.InitSetup()

	case "complete_setup":
		if err := validateCompleteSetup(p.ProjectDir, p.TemplateDir); err != nil {
			return "", err
		}
		result, err := m.projectHandler.CompleteSetup(p.ProjectDir, p.TemplateDir)
		if err != nil {
			return "", err
		}

		// Expand tilde in paths before persisting
		expandedProjectDir, err := expandTilde(p.ProjectDir)
		if err != nil {
			fmt.Printf("Warning: Failed to expand project directory path: %v\n", err)
			expandedProjectDir = p.ProjectDir
		}

		expandedTemplateDir, err := expandTilde(p.TemplateDir)
		if err != nil {
			fmt.Printf("Warning: Failed to expand template directory path: %v\n", err)
			expandedTemplateDir = p.TemplateDir
		}

		// Update settings manager to persist changes
		if m.settings != nil {
			fmt.Printf("DEBUG: About to call UpdateSettings with agentContext: %+v\n", m.agentContext)
			if updateErr := m.settings.UpdateSettings(expandedProjectDir, expandedTemplateDir, true, m.agentContext); updateErr != nil {
				// Log the error but don't fail the operation
				fmt.Printf("Warning: Failed to update settings: %v\n", updateErr)
			} else {
				fmt.Printf("DEBUG: Settings updated successfully\n")
			}
		} else {
			fmt.Printf("DEBUG: settings is nil\n")
		}

		return result, nil

	default:
		return "", fmt.Errorf("unknown operation %q. Valid operations: create_project, set_project_dir, set_template_dir, get_settings, init_setup, complete_setup", p.Operation)
	}
}

// Settings interface implementation
func (m *musicProjectManagerTool) GetSettings() (string, error) {
	if m.settings == nil {
		m.settings = &SettingsManager{}
	}
	return m.settings.GetSettings()
}

func (m *musicProjectManagerTool) SetSettings(settings string) error {
	if m.settings == nil {
		m.settings = &SettingsManager{}
	}
	return m.settings.SetSettings(settings)
}

func (m *musicProjectManagerTool) GetDefaultSettings() (string, error) {
	if m.settings == nil {
		m.settings = &SettingsManager{}
	}
	return m.settings.GetDefaultSettings()
}

func (m *musicProjectManagerTool) IsInitialized() bool {
	if m.settings == nil {
		m.settings = &SettingsManager{}
	}
	return m.settings.IsInitialized()
}

// SetAgentContext provides the current agent information to the plugin
func (m *musicProjectManagerTool) SetAgentContext(ctx pluginapi.AgentContext) {
	m.agentContext = &ctx
	// Reset project handler to use new context
	m.projectHandler = nil
}

// Validation functions

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

// validatePath validates a file system path parameter
func validatePath(path, pathType string) error {
	if path == "" {
		return fmt.Errorf("%s path is required and cannot be empty", pathType)
	}

	// Expand tilde first
	expandedPath, err := expandTilde(path)
	if err != nil {
		return fmt.Errorf("failed to expand home directory in %s path %q: %w", pathType, path, err)
	}

	// Convert to absolute path and check if it's valid
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return fmt.Errorf("invalid %s path %q: %w", pathType, path, err)
	}

	// Check if parent directory exists (for the case where we're creating the final directory)
	parentDir := filepath.Dir(absPath)
	if _, err := os.Stat(parentDir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("parent directory for %s does not exist: %s", pathType, parentDir)
		}
		return fmt.Errorf("cannot access parent directory for %s: %w", pathType, err)
	}

	return nil
}

// validateCompleteSetup validates parameters for complete_setup operation
func validateCompleteSetup(projectDir, templateDir string) error {
	if projectDir == "" {
		return fmt.Errorf("project_dir is required for complete_setup operation")
	}

	if templateDir == "" {
		return fmt.Errorf("template_dir is required for complete_setup operation")
	}

	// Validate both paths
	if err := validatePath(projectDir, "project directory"); err != nil {
		return err
	}

	if err := validatePath(templateDir, "template directory"); err != nil {
		return err
	}

	// Check that the directories are different
	absProjectDir, _ := filepath.Abs(projectDir)
	absTemplateDir, _ := filepath.Abs(templateDir)

	if absProjectDir == absTemplateDir {
		return fmt.Errorf("project directory and template directory cannot be the same")
	}

	return nil
}

// Tool is the exported symbol that the host application will look up.
var Tool = musicProjectManagerTool{}
