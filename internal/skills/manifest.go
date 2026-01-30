package skills

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const managedManifestFile = ".controlccx_skill.json"

const (
	sourceTypeLocal  = "local"
	sourceTypeGit    = "git"
	sourceTypeImport = "import"
)

type ManagedSkillManifest struct {
	SchemaVersion int    `json:"schema_version"`
	Name          string `json:"name"`

	SourceType     string `json:"source_type,omitempty"`
	SourceTool     string `json:"source_tool,omitempty"`
	SourceRef      string `json:"source_ref,omitempty"`
	SourceBranch   string `json:"source_branch,omitempty"`
	SourceSubpath  string `json:"source_subpath,omitempty"`
	SourceRevision string `json:"source_revision,omitempty"`

	ContentHash string `json:"content_hash,omitempty"`

	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func readManagedManifest(skillDir string) (ManagedSkillManifest, error) {
	path := filepath.Join(filepath.Clean(skillDir), managedManifestFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return ManagedSkillManifest{}, err
	}
	var out ManagedSkillManifest
	if err := json.Unmarshal(data, &out); err != nil {
		return ManagedSkillManifest{}, err
	}
	return out, nil
}

func writeManagedManifest(skillDir string, m ManagedSkillManifest) error {
	skillDir = filepath.Clean(skillDir)
	if skillDir == "" {
		return fmt.Errorf("skills: write manifest: empty skill dir")
	}

	m.SchemaVersion = 1
	m.Name = strings.TrimSpace(m.Name)
	m.SourceType = strings.TrimSpace(m.SourceType)
	m.SourceTool = strings.TrimSpace(m.SourceTool)
	m.SourceRef = strings.TrimSpace(m.SourceRef)
	m.SourceBranch = strings.TrimSpace(m.SourceBranch)
	m.SourceSubpath = strings.TrimSpace(m.SourceSubpath)
	m.SourceRevision = strings.TrimSpace(m.SourceRevision)
	m.ContentHash = strings.TrimSpace(m.ContentHash)

	now := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(m.CreatedAt) == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("skills: marshal manifest: %w", err)
	}
	data = append(data, '\n')
	path := filepath.Join(skillDir, managedManifestFile)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("skills: write manifest: %w", err)
	}
	return nil
}
