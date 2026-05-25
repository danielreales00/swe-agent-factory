package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/danielreales00/swe-agent-factory/internal/config"
)

// Registry is the in-memory set of bridge Targets loaded from targets/*.yml
// at startup. Lookups are case-sensitive by file basename (e.g. the file
// `targets/personal_finance.yml` registers a target named "personal_finance").
type Registry struct {
	targets map[string]Target
	order   []string // alphabetical for stable Default + listing
}

// LoadRegistry scans targetsDir for *.yml files, parses each via
// config.LoadTarget, resolves them with TargetFromDescriptor, and indexes
// the results. Errors if the directory is missing, contains no targets, or
// any single target fails to resolve (so misconfiguration is loud at boot,
// not when a /repo command lands).
func LoadRegistry(targetsDir string) (*Registry, error) {
	entries, err := os.ReadDir(targetsDir)
	if err != nil {
		return nil, fmt.Errorf("read targets dir %s: %w", targetsDir, err)
	}

	reg := &Registry{targets: map[string]Target{}}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml")
		td, err := config.LoadTarget(targetsDir, base)
		if err != nil {
			return nil, fmt.Errorf("load %s: %w", filepath.Join(targetsDir, name), err)
		}
		t, err := TargetFromDescriptor(base, td)
		if err != nil {
			return nil, err
		}
		reg.targets[base] = t
		reg.order = append(reg.order, base)
	}
	if len(reg.targets) == 0 {
		return nil, fmt.Errorf("no targets found in %s", targetsDir)
	}
	sort.Strings(reg.order)
	return reg, nil
}

// Get returns the named Target. Second return is false if absent.
func (r *Registry) Get(name string) (Target, bool) {
	t, ok := r.targets[name]
	return t, ok
}

// Names returns target names in alphabetical order.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// Default is the target used when a new chat session has no /repo
// selection yet. Currently the alphabetically first registered target;
// chosen for determinism so adding a target doesn't silently change
// existing chats.
func (r *Registry) Default() Target {
	if len(r.order) == 0 {
		return Target{}
	}
	return r.targets[r.order[0]]
}

// All returns a slice of every registered target, in alphabetical order.
// Used by main.go to GC worktrees across every target at boot.
func (r *Registry) All() []Target {
	out := make([]Target, 0, len(r.order))
	for _, n := range r.order {
		out = append(out, r.targets[n])
	}
	return out
}

// ErrUnknownTarget is what /repo <name> returns when name isn't in the registry.
var ErrUnknownTarget = errors.New("unknown target")
