package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/danielreales00/swe-agent-factory/internal/config"
)

// Target is the bridge's runtime view of a target repo: where it lives,
// which branch worktrees fork from, where worktrees should be created.
//
// 2.8 loads multiple Targets from targets/*.yml via Registry. Sessions
// pick one with /repo and may switch between tasks.
type Target struct {
	Name          string // human-readable, e.g. "personal_finance"
	RepoPath      string // absolute path to the live repo
	DefaultBranch string // branch worktrees fork from, e.g. "main"
	WorktreeRoot  string // dir where worktrees are created (sibling of RepoPath)
	BranchPrefix  string // placeholder branches use this prefix, e.g. "bridge/"
}

// TargetFromDescriptor resolves a parsed targets/<name>.yml into a runtime
// Target. Verifies the repo exists locally; applies defaults for branch /
// worktree root / branch prefix.
func TargetFromDescriptor(name string, td *config.TargetDescriptor) (Target, error) {
	if name == "" {
		return Target{}, errors.New("target name is required")
	}
	if td == nil {
		return Target{}, errors.New("nil descriptor")
	}
	if td.LocalPath == "" {
		return Target{}, fmt.Errorf("target %q: local_path is required", name)
	}
	abs, err := filepath.Abs(td.LocalPath)
	if err != nil {
		return Target{}, fmt.Errorf("target %q: resolve local_path: %w", name, err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return Target{}, fmt.Errorf("target %q: not a git repo: %s", name, abs)
	}

	branch := td.DefaultBranch
	if branch == "" {
		branch = "main"
	}
	wtRoot := td.WorktreeRoot
	if wtRoot == "" {
		wtRoot = filepath.Dir(abs)
	} else {
		wtRoot, err = filepath.Abs(wtRoot)
		if err != nil {
			return Target{}, fmt.Errorf("target %q: resolve worktree_root: %w", name, err)
		}
	}
	prefix := td.BranchPrefix
	if prefix == "" {
		prefix = "bridge/"
	}

	return Target{
		Name:          name,
		RepoPath:      abs,
		DefaultBranch: branch,
		WorktreeRoot:  wtRoot,
		BranchPrefix:  prefix,
	}, nil
}
