package bridge

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTargetYML writes a targets/<name>.yml that points at repoPath.
func writeTargetYML(t *testing.T, targetsDir, name, repoPath string) {
	t.Helper()
	body := fmt.Sprintf("local_path: %s\nrepo: git@example.com:x/%s.git\nmanifest_path: .swe-agent.yml\n",
		repoPath, name)
	if err := os.WriteFile(filepath.Join(targetsDir, name+".yml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s.yml: %v", name, err)
	}
}

func TestLoadRegistry_HappyPath(t *testing.T) {
	repoA := setupGitRepo(t)
	repoB := setupGitRepo(t)
	dir := t.TempDir()
	writeTargetYML(t, dir, "alpha", repoA)
	writeTargetYML(t, dir, "beta", repoB)
	// non-yml files are ignored
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	names := reg.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("Names = %v, want [alpha beta]", names)
	}
	if reg.Default().Name != "alpha" {
		t.Errorf("Default = %q, want alpha (alphabetical first)", reg.Default().Name)
	}
	if t2, ok := reg.Get("beta"); !ok || t2.RepoPath != repoB {
		t.Errorf("Get(beta) = (%+v, %v)", t2, ok)
	}
	if _, ok := reg.Get("missing"); ok {
		t.Error("Get(missing) ok=true, want false")
	}
}

func TestLoadRegistry_MissingDir(t *testing.T) {
	_, err := LoadRegistry(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("want error")
	}
}

func TestLoadRegistry_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadRegistry(dir)
	if err == nil || !strings.Contains(err.Error(), "no targets") {
		t.Errorf("err = %v, want 'no targets'", err)
	}
}

func TestLoadRegistry_BadYAML(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.yml"), []byte("not: [valid"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadRegistry(dir)
	if err == nil {
		t.Fatal("want parse error")
	}
}

func TestLoadRegistry_TargetWithMissingLocalPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.yml"),
		[]byte("repo: git@example.com:x/x.git\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadRegistry(dir)
	if err == nil || !strings.Contains(err.Error(), "local_path") {
		t.Errorf("err = %v, want local_path complaint", err)
	}
}
