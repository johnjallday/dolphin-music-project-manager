package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/johnjallday/music_project_manager/common"
	"github.com/johnjallday/music_project_manager/handlers"
	"github.com/openai/openai-go/v2"
)


// musicProjectManagerTool implements Tool for music project management.
type musicProjectManagerTool struct {
	agentContext    *common.AgentContext
	projectHandler  *handlers.ProjectHandler
	settings        *SettingsManager
}

func (m *musicProjectManagerTool) Version() string {
	versionBytes, err := os.ReadFile("VERSION")
	if err != nil {
		return "0.0.1"
	}
	return strings.TrimSpace(string(versionBytes))
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
		return m.projectHandler.SetProjectDir(p.Path)

	case "set_template_dir":
		if err := validatePath(p.Path, "template directory"); err != nil {
			return "", err
		}
		return m.projectHandler.SetTemplateDir(p.Path)

	case "get_settings":
		return m.projectHandler.GetSettings()

	case "init_setup":
		return m.projectHandler.InitSetup()

	case "complete_setup":
		if err := validateCompleteSetup(p.ProjectDir, p.TemplateDir); err != nil {
			return "", err
		}
		return m.projectHandler.CompleteSetup(p.ProjectDir, p.TemplateDir)

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
func (m *musicProjectManagerTool) SetAgentContext(ctx common.AgentContext) {
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

// validatePath validates a file system path parameter
func validatePath(path, pathType string) error {
	if path == "" {
		return fmt.Errorf("%s path is required and cannot be empty", pathType)
	}
	
	// Convert to absolute path and check if it's valid
	absPath, err := filepath.Abs(path)
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
