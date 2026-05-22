package signal

import (
	"context"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/workitem"
)

// Window narrows what a Source returns. Sources may ignore fields that
// don't apply (e.g. inline ignores all of them).
type Window struct {
	Since time.Time
	Until time.Time
}

// Source is the pluggable input side of the factory. Each kind in
// `signals:` (axiom, linear, github, inline, manifest) implements this.
//
// Returns are the *raw* observations; the pipeline handles dedup against
// the state store. Sources may return zero items without erroring.
type Source interface {
	// Name is the manifest's signals[].name, or the kind if unnamed.
	Name() string
	// Kind matches manifest signals[].kind ("axiom", "linear", ...).
	Kind() string
	Collect(ctx context.Context, w Window) ([]workitem.WorkItem, error)
}
