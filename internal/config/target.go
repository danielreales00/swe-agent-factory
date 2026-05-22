package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TargetDescriptor lives at targets/<name>.yml. Tells the factory how to
// fetch the target repo and where its manifest lives inside.
type TargetDescriptor struct {
	Repo         string `yaml:"repo"`
	ManifestPath string `yaml:"manifest_path"`
}

// Manifest mirrors .swe-agent.yml. Only fields the factory actually reads
// in Phase 1 are typed; the rest can be added incrementally.
type Manifest struct {
	Version int `yaml:"version"`

	Project struct {
		Name        string `yaml:"name"`
		Language    string `yaml:"language"`
		Description string `yaml:"description"`
	} `yaml:"project"`

	Conventions struct {
		VoiceFile string `yaml:"voice_file"`
	} `yaml:"conventions"`

	Specs struct {
		Dir       string `yaml:"dir"`
		Roadmap   string `yaml:"roadmap"`
		Exemplar  string `yaml:"exemplar"`
		Numbering string `yaml:"numbering"`
		Format    string `yaml:"format"`
	} `yaml:"specs"`

	Build struct {
		FmtCmd         string   `yaml:"fmt_cmd"`
		QualityCmd     string   `yaml:"quality_cmd"`
		IntegrationCmd string   `yaml:"integration_cmd"`
		PreCommit      []string `yaml:"pre_commit"`
	} `yaml:"build"`

	SourceLayout map[string]string `yaml:"source_layout"`

	Signals []Signal `yaml:"signals"`

	Git struct {
		DefaultBranch string `yaml:"default_branch"`
		BranchPrefix  string `yaml:"branch_prefix"`
		CommitSigning bool   `yaml:"commit_signing"`
	} `yaml:"git"`

	Linear struct {
		Team    *string `yaml:"team"`
		Project *string `yaml:"project"`
		Label   string  `yaml:"label"`
	} `yaml:"linear"`

	Approvers []string `yaml:"approvers"`

	Testing struct {
		UnitOnly          string `yaml:"unit_only"`
		CoverageThreshold int    `yaml:"coverage_threshold"`
		MocksFile         string `yaml:"mocks_file"`
	} `yaml:"testing"`
}

// Signal is one entry under `signals:`. Different kinds use different
// subset of fields; consumer-side type assertion via Kind.
type Signal struct {
	Kind      string `yaml:"kind"`
	Name      string `yaml:"name"`
	Dataset   string `yaml:"dataset"`
	Query     string `yaml:"query"`
	ClusterBy string `yaml:"cluster_by"`
	TriageAs  string `yaml:"triage_as"`

	// linear
	Team  string `yaml:"team"`
	Label string `yaml:"label"`

	// github
	Repo   string   `yaml:"repo"`
	Labels []string `yaml:"labels"`

	// manifest
	Items []struct {
		Title       string `yaml:"title"`
		Description string `yaml:"description"`
	} `yaml:"items"`
}

func LoadTarget(targetsDir, name string) (*TargetDescriptor, error) {
	path := filepath.Join(targetsDir, name+".yml")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read target %s: %w", path, err)
	}
	var td TargetDescriptor
	if err := yaml.Unmarshal(b, &td); err != nil {
		return nil, fmt.Errorf("parse target %s: %w", path, err)
	}
	return &td, nil
}

func LoadManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if m.Version != 1 {
		return nil, fmt.Errorf("unsupported manifest version: %d", m.Version)
	}
	return &m, nil
}
