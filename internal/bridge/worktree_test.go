package bridge

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func targetFromRepo(repo string) Target {
	return Target{
		Name:          "test",
		RepoPath:      repo,
		DefaultBranch: "main",
		WorktreeRoot:  filepath.Dir(repo),
		BranchPrefix:  "bridge/",
	}
}

func TestWorktreeManager_CreateAndRemove(t *testing.T) {
	repo := setupGitRepo(t)
	mgr := NewWorktreeManager()
	target := targetFromRepo(repo)
	ctx := context.Background()

	path, branch, err := mgr.Create(ctx, target, 12345)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(path), "repo-tg-12345-") {
		t.Errorf("worktree dir = %q, want prefix repo-tg-12345-", filepath.Base(path))
	}
	if !strings.Contains(branch, "_pending-tg-12345-") {
		t.Errorf("branch = %q, want _pending-tg-12345- substring", branch)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("worktree dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(path, "README.md")); err != nil {
		t.Errorf("repo content not present in worktree: %v", err)
	}
	if branchExists(t, repo, branch) != true {
		t.Errorf("placeholder branch %q missing after Create", branch)
	}

	if err := mgr.Remove(ctx, target, path, branch); err != nil {
		t.Errorf("Remove: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("worktree dir still exists after Remove")
	}
	if branchExists(t, repo, branch) {
		t.Errorf("placeholder branch %q still exists after Remove", branch)
	}
}

func TestWorktreeManager_GC_RemovesStaleOnly(t *testing.T) {
	repo := setupGitRepo(t)
	mgr := NewWorktreeManager()
	target := targetFromRepo(repo)
	ctx := context.Background()

	pathOld, _, err := mgr.Create(ctx, target, 1)
	if err != nil {
		t.Fatalf("Create old: %v", err)
	}
	pathNew, _, err := mgr.Create(ctx, target, 2)
	if err != nil {
		t.Fatalf("Create new: %v", err)
	}

	// Backdate the first worktree by 48h so it falls outside the 24h window.
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(pathOld, twoDaysAgo, twoDaysAgo); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	n, err := mgr.GC(ctx, target, 24*time.Hour)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if n != 1 {
		t.Errorf("GC cleaned %d, want 1", n)
	}
	if _, err := os.Stat(pathOld); err == nil {
		t.Errorf("stale worktree was not removed")
	}
	if _, err := os.Stat(pathNew); err != nil {
		t.Errorf("recent worktree was removed: %v", err)
	}
}

func TestWorktreeManager_GC_IgnoresUnrelatedDirs(t *testing.T) {
	repo := setupGitRepo(t)
	mgr := NewWorktreeManager()
	target := targetFromRepo(repo)
	ctx := context.Background()

	unrelated := filepath.Join(filepath.Dir(repo), "some-other-dir")
	if err := os.Mkdir(unrelated, 0o755); err != nil {
		t.Fatalf("mkdir unrelated: %v", err)
	}
	twoDaysAgo := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(unrelated, twoDaysAgo, twoDaysAgo); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	n, err := mgr.GC(ctx, target, 24*time.Hour)
	if err != nil {
		t.Fatalf("GC: %v", err)
	}
	if n != 0 {
		t.Errorf("GC cleaned %d unrelated dirs, want 0", n)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Errorf("unrelated dir was removed: %v", err)
	}
}

func TestWorktreeManager_BranchEncodesChatID(t *testing.T) {
	repo := setupGitRepo(t)
	mgr := NewWorktreeManager()
	target := targetFromRepo(repo)

	_, branch1, err := mgr.Create(context.Background(), target, 99)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, branch2, err := mgr.Create(context.Background(), target, 100)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.Contains(branch1, "-99-") {
		t.Errorf("branch1 = %q, want chat id 99 in it", branch1)
	}
	if !strings.Contains(branch2, "-100-") {
		t.Errorf("branch2 = %q, want chat id 100 in it", branch2)
	}
	if branch1 == branch2 {
		t.Errorf("branches collided: %q", branch1)
	}
}

func branchExists(t *testing.T, repo, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "branch", "--list", branch)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git branch --list: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}
