package bridge

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielreales00/swe-agent-factory/internal/config"
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

func TestTargetFromDescriptor_HappyPath_Defaults(t *testing.T) {
	repo := setupGitRepo(t)
	td := &config.TargetDescriptor{LocalPath: repo}

	target, err := TargetFromDescriptor("test_target", td)
	if err != nil {
		t.Fatalf("TargetFromDescriptor: %v", err)
	}
	if target.Name != "test_target" {
		t.Errorf("Name = %q", target.Name)
	}
	if target.RepoPath != repo {
		t.Errorf("RepoPath = %q, want %q", target.RepoPath, repo)
	}
	if target.DefaultBranch != "main" {
		t.Errorf("DefaultBranch = %q, want main", target.DefaultBranch)
	}
	if target.WorktreeRoot != filepath.Dir(repo) {
		t.Errorf("WorktreeRoot = %q, want parent of repo", target.WorktreeRoot)
	}
	if target.BranchPrefix != "bridge/" {
		t.Errorf("BranchPrefix = %q", target.BranchPrefix)
	}
}

func TestTargetFromDescriptor_OverrideDefaults(t *testing.T) {
	repo := setupGitRepo(t)
	customRoot := t.TempDir()
	td := &config.TargetDescriptor{
		LocalPath:     repo,
		DefaultBranch: "trunk",
		WorktreeRoot:  customRoot,
		BranchPrefix:  "tg/",
	}
	target, err := TargetFromDescriptor("x", td)
	if err != nil {
		t.Fatalf("TargetFromDescriptor: %v", err)
	}
	if target.DefaultBranch != "trunk" || target.WorktreeRoot != customRoot || target.BranchPrefix != "tg/" {
		t.Errorf("overrides not applied: %+v", target)
	}
}

func TestTargetFromDescriptor_EmptyName(t *testing.T) {
	repo := setupGitRepo(t)
	_, err := TargetFromDescriptor("", &config.TargetDescriptor{LocalPath: repo})
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("err = %v, want name error", err)
	}
}

func TestTargetFromDescriptor_NilDescriptor(t *testing.T) {
	_, err := TargetFromDescriptor("x", nil)
	if err == nil {
		t.Fatal("want error")
	}
}

func TestTargetFromDescriptor_MissingLocalPath(t *testing.T) {
	_, err := TargetFromDescriptor("x", &config.TargetDescriptor{})
	if err == nil || !strings.Contains(err.Error(), "local_path") {
		t.Errorf("err = %v, want local_path error", err)
	}
}

func TestTargetFromDescriptor_NotGitRepo(t *testing.T) {
	_, err := TargetFromDescriptor("x", &config.TargetDescriptor{LocalPath: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "not a git repo") {
		t.Errorf("err = %v, want not-a-git-repo error", err)
	}
}
