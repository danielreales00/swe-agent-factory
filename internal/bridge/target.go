package bridge

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Target is the bridge's runtime view of a target repo: where it lives,
// which branch worktrees fork from, where worktrees should be created.
//
// 2.3 loads one target from env vars. 2.8 will load multiple from
// targets/*.yml via /repo selection.
type Target struct {
	Name          string // human-readable, e.g. "personal_finance"
	RepoPath      string // absolute path to the live repo
	DefaultBranch string // branch worktrees fork from, e.g. "main"
	WorktreeRoot  string // dir where worktrees are created (sibling of RepoPath)
	BranchPrefix  string // placeholder branches use this prefix, e.g. "bridge/"
}

// TargetFromEnv builds a Target from BRIDGE_TARGET_* env vars.
//
// Required:
//
//	BRIDGE_TARGET_NAME       (e.g. "personal_finance")
//	BRIDGE_TARGET_REPO_PATH  (absolute, must contain a .git entry)
//
// Optional:
//
//	BRIDGE_TARGET_DEFAULT_BRANCH  default: "main"
//	BRIDGE_TARGET_WORKTREE_ROOT   default: parent dir of RepoPath
//	BRIDGE_TARGET_BRANCH_PREFIX   default: "bridge/"
func TargetFromEnv() (Target, error) {
	name := os.Getenv("BRIDGE_TARGET_NAME")
	if name == "" {
		return Target{}, errors.New("BRIDGE_TARGET_NAME is required")
	}
	path := os.Getenv("BRIDGE_TARGET_REPO_PATH")
	if path == "" {
		return Target{}, errors.New("BRIDGE_TARGET_REPO_PATH is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return Target{}, fmt.Errorf("resolve repo path: %w", err)
	}
	if _, err := os.Stat(filepath.Join(abs, ".git")); err != nil {
		return Target{}, fmt.Errorf("not a git repo: %s", abs)
	}

	branch := os.Getenv("BRIDGE_TARGET_DEFAULT_BRANCH")
	if branch == "" {
		branch = "main"
	}
	wtRoot := os.Getenv("BRIDGE_TARGET_WORKTREE_ROOT")
	if wtRoot == "" {
		wtRoot = filepath.Dir(abs)
	} else {
		wtRoot, err = filepath.Abs(wtRoot)
		if err != nil {
			return Target{}, fmt.Errorf("resolve worktree root: %w", err)
		}
	}
	prefix := os.Getenv("BRIDGE_TARGET_BRANCH_PREFIX")
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
