package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// WorktreeManager is target-agnostic: each Create/Remove/GC call carries
// the Target it's acting on. This lets one manager serve a multi-target
// registry without holding per-target state.
type WorktreeManager struct{}

func NewWorktreeManager() *WorktreeManager { return &WorktreeManager{} }

// Create makes a fresh worktree off the target's default branch on a new
// placeholder branch. Returns absolute worktree path + branch name.
func (m *WorktreeManager) Create(ctx context.Context, t Target, chatID int64) (path, branch string, err error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	repoBase := filepath.Base(t.RepoPath)
	path = filepath.Join(t.WorktreeRoot, fmt.Sprintf("%s-tg-%d-%s", repoBase, chatID, ts))
	branch = fmt.Sprintf("%s_pending-tg-%d-%s", t.BranchPrefix, chatID, ts)

	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-b", branch, path, t.DefaultBranch)
	cmd.Dir = t.RepoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w (%s)",
			err, strings.TrimSpace(stderr.String()))
	}
	return path, branch, nil
}

// Remove deletes the worktree on disk and the placeholder branch. Branch
// deletion is best-effort — if the branch was renamed (e.g. ship flow
// renamed _pending to swe-agent/<slug>), the original is gone and that's OK.
func (m *WorktreeManager) Remove(ctx context.Context, t Target, path, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
	cmd.Dir = t.RepoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w (%s)",
			err, strings.TrimSpace(stderr.String()))
	}
	if branch != "" {
		del := exec.CommandContext(ctx, "git", "branch", "-D", branch)
		del.Dir = t.RepoPath
		_ = del.Run() // best effort
	}
	return nil
}

// GC removes bridge-created worktrees in t.WorktreeRoot whose mtime is
// older than maxAge. Used on bridge startup to clean up after crashes.
//
// Returns the number of worktrees cleaned. A trailing `git worktree prune`
// reaps the metadata regardless.
func (m *WorktreeManager) GC(ctx context.Context, t Target, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(t.WorktreeRoot)
	if err != nil {
		return 0, fmt.Errorf("read worktree root: %w", err)
	}

	prefix := filepath.Base(t.RepoPath) + "-tg-"
	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(t.WorktreeRoot, e.Name())
		cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
		cmd.Dir = t.RepoPath
		if err := cmd.Run(); err != nil {
			_ = os.RemoveAll(path)
		}
		cleaned++
	}

	if cleaned > 0 {
		prune := exec.CommandContext(ctx, "git", "worktree", "prune")
		prune.Dir = t.RepoPath
		_ = prune.Run()
	}
	return cleaned, nil
}
