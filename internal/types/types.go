package types

import "time"

// MusicProjectParams defines the parameters using struct tags.
type MusicProjectParams struct {
	Operation string `json:"operation" description:"Music project operation: create new Reaper project, open existing project in Reaper DAW, reveal project in Finder file browser, scan for .RPP files, list projects, filter by name/BPM, or rename an existing project" enum:"create_project,scan,list_projects,open_project,open_in_finder,filter_project,rename_project" required:"true"`
	Name      string `json:"name" description:"Project name for creating new Reaper projects, filtering existing ones, finding projects to open in Finder, or the current name of a project to rename (e.g., 'mash', 'beats', 'Rich Daddy', 'China girl EDM')"`
	NewName   string `json:"new_name" description:"New name for the project when using rename_project operation (e.g., 'okok')"`
	Path      string `json:"path" description:"Full file path to a Reaper project file (.RPP) to open in Reaper DAW or reveal in Finder (e.g., '/Users/name/Music/Projects/song.RPP')"`
	BPM       int    `json:"bpm" description:"BPM for the project (optional for create_project, exact BPM for filter_project)" min:"30" max:"300"`
	MinBPM    int    `json:"min_bpm" description:"Minimum BPM for filter_project (optional)" min:"30" max:"300"`
	MaxBPM    int    `json:"max_bpm" description:"Maximum BPM for filter_project (optional)" min:"30" max:"300"`
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
