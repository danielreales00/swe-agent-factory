package workitem

import "time"

type Type string

const (
	TypeCapabilityGap   Type = "capability_gap"
	TypeFeatureRequest  Type = "feature_request"
	TypeBugReport       Type = "bug_report"
	TypeInlineTask      Type = "inline_task"
	TypeManifestFeature Type = "manifest_feature"
)

type SourceID string

const (
	SourceAxiom    SourceID = "axiom"
	SourceLinear   SourceID = "linear"
	SourceGitHub   SourceID = "github"
	SourceInline   SourceID = "inline"
	SourceManifest SourceID = "manifest"
)

type Evidence struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
}

type Status string

const (
	StatusOpen    Status = "open"
	StatusShipped Status = "shipped"
	StatusClosed  Status = "closed"
)

type WorkItem struct {
	ID          string            `json:"id"`
	Type        Type              `json:"type"`
	Source      SourceID          `json:"source"`
	Target      string            `json:"target"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Evidence    []Evidence        `json:"evidence,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Status      Status            `json:"status,omitempty"`
	PRUrl       string            `json:"pr_url,omitempty"`
}
