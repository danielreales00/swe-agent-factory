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

// WorktreeManager creates, removes, and garbage-collects git worktrees for
// a target repo. Worktrees are named `<repo-basename>-tg-<chatID>-<ts>` so
// `git worktree list` shows the chat correlation directly.
type WorktreeManager struct {
	target Target
}

func NewWorktreeManager(t Target) *WorktreeManager {
	return &WorktreeManager{target: t}
}

// Create makes a fresh worktree off the target's default branch on a new
// placeholder branch. Returns absolute worktree path + branch name.
func (m *WorktreeManager) Create(ctx context.Context, chatID int64) (path, branch string, err error) {
	ts := time.Now().UTC().Format("20060102T150405Z")
	repoBase := filepath.Base(m.target.RepoPath)
	path = filepath.Join(m.target.WorktreeRoot, fmt.Sprintf("%s-tg-%d-%s", repoBase, chatID, ts))
	branch = fmt.Sprintf("%s_pending-tg-%d-%s", m.target.BranchPrefix, chatID, ts)

	cmd := exec.CommandContext(ctx, "git", "worktree", "add",
		"-b", branch, path, m.target.DefaultBranch)
	cmd.Dir = m.target.RepoPath
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
func (m *WorktreeManager) Remove(ctx context.Context, path, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
	cmd.Dir = m.target.RepoPath
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git worktree remove: %w (%s)",
			err, strings.TrimSpace(stderr.String()))
	}
	if branch != "" {
		del := exec.CommandContext(ctx, "git", "branch", "-D", branch)
		del.Dir = m.target.RepoPath
		_ = del.Run() // best effort
	}
	return nil
}

// GC removes bridge-created worktrees in WorktreeRoot whose mtime is older
// than maxAge. Used on bridge startup to clean up after crashes.
//
// Returns the number of worktrees cleaned. A trailing `git worktree prune`
// reaps the metadata regardless.
func (m *WorktreeManager) GC(ctx context.Context, maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(m.target.WorktreeRoot)
	if err != nil {
		return 0, fmt.Errorf("read worktree root: %w", err)
	}

	prefix := filepath.Base(m.target.RepoPath) + "-tg-"
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
		path := filepath.Join(m.target.WorktreeRoot, e.Name())
		cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", path)
		cmd.Dir = m.target.RepoPath
		if err := cmd.Run(); err != nil {
			// Worktree metadata may be corrupted; force-remove the dir
			// and let `prune` reap the ref below.
			_ = os.RemoveAll(path)
		}
		cleaned++
	}

	if cleaned > 0 {
		prune := exec.CommandContext(ctx, "git", "worktree", "prune")
		prune.Dir = m.target.RepoPath
		_ = prune.Run()
	}
	return cleaned, nil
}
