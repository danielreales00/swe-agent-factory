package bridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupGitRepo creates a fresh git repo in a temp dir with one initial
// commit on the default branch. Returns the repo's absolute path.
func setupGitRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitEnv := append(os.Environ(),
		"GIT_AUTHOR_NAME=test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = gitEnv
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "initial")
	return repo
}

func TestTargetFromEnv_HappyPath(t *testing.T) {
	repo := setupGitRepo(t)
	t.Setenv("BRIDGE_TARGET_NAME", "test_target")
	t.Setenv("BRIDGE_TARGET_REPO_PATH", repo)
	t.Setenv("BRIDGE_TARGET_DEFAULT_BRANCH", "")
	t.Setenv("BRIDGE_TARGET_WORKTREE_ROOT", "")
	t.Setenv("BRIDGE_TARGET_BRANCH_PREFIX", "")

	target, err := TargetFromEnv()
	if err != nil {
		t.Fatalf("TargetFromEnv: %v", err)
	}
	if target.Name != "test_target" {
		t.Errorf("Name = %q, want test_target", target.Name)
	}
	if target.RepoPath != repo {
		t.Errorf("RepoPath = %q, want %q", target.RepoPath, repo)
	}
	if target.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main (default)", target.DefaultBranch)
	}
	if target.WorktreeRoot != filepath.Dir(repo) {
		t.Errorf("WorktreeRoot = %q, want parent of repo", target.WorktreeRoot)
	}
	if target.BranchPrefix != "bridge/" {
		t.Errorf("BranchPrefix = %q, want bridge/", target.BranchPrefix)
	}
}

func TestTargetFromEnv_OverrideDefaults(t *testing.T) {
	repo := setupGitRepo(t)
	customRoot := t.TempDir()
	t.Setenv("BRIDGE_TARGET_NAME", "x")
	t.Setenv("BRIDGE_TARGET_REPO_PATH", repo)
	t.Setenv("BRIDGE_TARGET_DEFAULT_BRANCH", "trunk")
	t.Setenv("BRIDGE_TARGET_WORKTREE_ROOT", customRoot)
	t.Setenv("BRIDGE_TARGET_BRANCH_PREFIX", "tg/")

	target, err := TargetFromEnv()
	if err != nil {
		t.Fatalf("TargetFromEnv: %v", err)
	}
	if target.DefaultBranch != "trunk" {
		t.Errorf("DefaultBranch = %q, want trunk", target.DefaultBranch)
	}
	if target.WorktreeRoot != customRoot {
		t.Errorf("WorktreeRoot = %q, want %q", target.WorktreeRoot, customRoot)
	}
	if target.BranchPrefix != "tg/" {
		t.Errorf("BranchPrefix = %q, want tg/", target.BranchPrefix)
	}
}

func TestTargetFromEnv_MissingName(t *testing.T) {
	t.Setenv("BRIDGE_TARGET_NAME", "")
	t.Setenv("BRIDGE_TARGET_REPO_PATH", "/tmp")
	_, err := TargetFromEnv()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "BRIDGE_TARGET_NAME") {
		t.Errorf("error %q doesn't mention BRIDGE_TARGET_NAME", err)
	}
}

func TestTargetFromEnv_MissingRepoPath(t *testing.T) {
	t.Setenv("BRIDGE_TARGET_NAME", "x")
	t.Setenv("BRIDGE_TARGET_REPO_PATH", "")
	_, err := TargetFromEnv()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "BRIDGE_TARGET_REPO_PATH") {
		t.Errorf("error %q doesn't mention BRIDGE_TARGET_REPO_PATH", err)
	}
}

func TestTargetFromEnv_NotGitRepo(t *testing.T) {
	t.Setenv("BRIDGE_TARGET_NAME", "x")
	t.Setenv("BRIDGE_TARGET_REPO_PATH", t.TempDir())
	_, err := TargetFromEnv()
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "not a git repo") {
		t.Errorf("error %q doesn't mention not-a-git-repo", err)
	}
}
