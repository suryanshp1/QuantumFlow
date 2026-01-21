package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ProjectManifest tracks all files created during plan execution
// This enables context propagation between phases and prevents overwrites
type ProjectManifest struct {
	ProjectName   string              `json:"project_name"`
	BaseDir       string              `json:"base_dir"`
	CreatedFiles  []FileEntry         `json:"created_files"`
	FileStructure map[string][]string `json:"file_structure"` // dir -> files
}

// FileEntry represents a single file created during execution
type FileEntry struct {
	Path      string    `json:"path"`
	Phase     string    `json:"phase"`
	Purpose   string    `json:"purpose"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

// NewProjectManifest creates a new manifest for tracking project files
func NewProjectManifest(projectName, baseDir string) *ProjectManifest {
	return &ProjectManifest{
		ProjectName:   projectName,
		BaseDir:       baseDir,
		CreatedFiles:  []FileEntry{},
		FileStructure: make(map[string][]string),
	}
}

// AddFile records a newly created file in the manifest
func (m *ProjectManifest) AddFile(path, phase, purpose string) {
	info, _ := os.Stat(path)
	var size int64
	if info != nil {
		size = info.Size()
	}

	m.CreatedFiles = append(m.CreatedFiles, FileEntry{
		Path:      path,
		Phase:     phase,
		Purpose:   purpose,
		Size:      size,
		CreatedAt: time.Now(),
	})

	// Update file structure map
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		dir = m.BaseDir
	}
	m.FileStructure[dir] = append(m.FileStructure[dir], filepath.Base(path))
}

// FileExists checks if a file was already created
func (m *ProjectManifest) FileExists(path string) bool {
	for _, f := range m.CreatedFiles {
		if f.Path == path {
			return true
		}
	}
	return false
}

// GetCreatedFilesForContext returns a formatted string listing created files
// for inclusion in LLM prompts
func (m *ProjectManifest) GetCreatedFilesForContext() string {
	if len(m.CreatedFiles) == 0 {
		return ""
	}

	result := "ALREADY CREATED FILES (do NOT overwrite):\n"
	for _, f := range m.CreatedFiles {
		result += fmt.Sprintf("  - %s (phase: %s)\n", f.Path, f.Phase)
	}
	return result
}

// GetFileStructureForContext returns a formatted string of expected file structure
func (m *ProjectManifest) GetFileStructureForContext() string {
	if len(m.FileStructure) == 0 {
		return ""
	}

	result := "PROJECT FILE STRUCTURE:\n"
	for dir, files := range m.FileStructure {
		result += fmt.Sprintf("  %s/\n", dir)
		for _, f := range files {
			result += fmt.Sprintf("    - %s\n", f)
		}
	}
	return result
}

// SetExpectedStructure sets the expected file structure from the plan
func (m *ProjectManifest) SetExpectedStructure(structure map[string][]string) {
	m.FileStructure = structure
}

// InitializeDirectories creates all directories in the file structure
func (m *ProjectManifest) InitializeDirectories() error {
	for dir := range m.FileStructure {
		if dir == "." || dir == "" {
			continue
		}
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Save writes the manifest to a JSON file
func (m *ProjectManifest) Save(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadManifest loads a manifest from a JSON file
func LoadManifest(path string) (*ProjectManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var manifest ProjectManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}
