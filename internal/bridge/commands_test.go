package bridge

import (
	"strings"
	"testing"

	"github.com/danielreales00/swe-agent-factory/internal/agent"
)

func TestParseCommand(t *testing.T) {
	cases := []struct {
		in   string
		want Command
	}{
		{"", Command{Kind: CmdNone}},
		{"hello", Command{Kind: CmdNone}},
		{"  hello", Command{Kind: CmdNone}},
		{"/repo", Command{Kind: CmdRepo}},
		{"/REPO", Command{Kind: CmdRepo}},
		{"/repo personal_finance", Command{Kind: CmdRepo, Arg: "personal_finance"}},
		{"/repo   spaced   ", Command{Kind: CmdRepo, Arg: "spaced"}},
		{"/cancel", Command{Kind: CmdCancel}},
		{"/Cancel", Command{Kind: CmdCancel}},
		{"/cancel ignored", Command{Kind: CmdCancel}},
		{"/unknown", Command{Kind: CmdUnknown, Arg: "unknown"}},
		{"/", Command{Kind: CmdUnknown}},
	}
	for _, c := range cases {
		got := ParseCommand(c.in)
		if got != c.want {
			t.Errorf("ParseCommand(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	repoA := setupGitRepo(t)
	repoB := setupGitRepo(t)
	dir := t.TempDir()
	writeTargetYML(t, dir, "alpha", repoA)
	writeTargetYML(t, dir, "beta", repoB)
	reg, err := LoadRegistry(dir)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	return reg
}

func TestExecuteCommand_RepoListIdle(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdRepo}, sess, reg)
	if !strings.Contains(r.Reply, "current: alpha") {
		t.Errorf("Reply = %q, want 'current: alpha'", r.Reply)
	}
	if !strings.Contains(r.Reply, "[alpha, beta]") {
		t.Errorf("Reply = %q, want available list", r.Reply)
	}
	if r.SwitchTarget != nil || r.Cancel {
		t.Errorf("unexpected effects: %+v", r)
	}
}

func TestExecuteCommand_RepoSwitchIdle(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdRepo, Arg: "beta"}, sess, reg)
	if !strings.Contains(r.Reply, "switched to beta") {
		t.Errorf("Reply = %q", r.Reply)
	}
	if r.SwitchTarget == nil || r.SwitchTarget.Name != "beta" {
		t.Errorf("SwitchTarget = %+v, want beta", r.SwitchTarget)
	}
}

func TestExecuteCommand_RepoSwitchSameTarget(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdRepo, Arg: "alpha"}, sess, reg)
	if !strings.Contains(r.Reply, "already on alpha") {
		t.Errorf("Reply = %q", r.Reply)
	}
	if r.SwitchTarget != nil {
		t.Errorf("SwitchTarget should be nil for same-target switch, got %+v", r.SwitchTarget)
	}
}

func TestExecuteCommand_RepoSwitchUnknown(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdRepo, Arg: "gamma"}, sess, reg)
	if !strings.Contains(r.Reply, "unknown target") {
		t.Errorf("Reply = %q", r.Reply)
	}
	if !strings.Contains(r.Reply, "[alpha, beta]") {
		t.Errorf("Reply = %q, want available list", r.Reply)
	}
	if r.SwitchTarget != nil {
		t.Error("SwitchTarget should be nil for unknown target")
	}
}

func TestExecuteCommand_RepoSwitchBlockedWhilePiAlive(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default(), Pi: &agent.Session{}}
	r := ExecuteCommand(Command{Kind: CmdRepo, Arg: "beta"}, sess, reg)
	if !strings.Contains(r.Reply, "/cancel first") {
		t.Errorf("Reply = %q, want /cancel hint", r.Reply)
	}
	if r.SwitchTarget != nil {
		t.Error("SwitchTarget should be nil when pi is alive")
	}
}

func TestExecuteCommand_CancelNoPi(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdCancel}, sess, reg)
	if !strings.Contains(r.Reply, "no pi running") {
		t.Errorf("Reply = %q", r.Reply)
	}
	if r.Cancel {
		t.Error("Cancel should be false with no pi")
	}
}

func TestExecuteCommand_CancelAlive(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default(), Pi: &agent.Session{}}
	r := ExecuteCommand(Command{Kind: CmdCancel}, sess, reg)
	if !strings.Contains(r.Reply, "cancelling") {
		t.Errorf("Reply = %q", r.Reply)
	}
	if !r.Cancel {
		t.Error("Cancel should be true with pi alive")
	}
}

func TestExecuteCommand_Unknown(t *testing.T) {
	reg := newTestRegistry(t)
	sess := &Session{Target: reg.Default()}
	r := ExecuteCommand(Command{Kind: CmdUnknown, Arg: "foo"}, sess, reg)
	if !strings.Contains(r.Reply, "unknown command /foo") {
		t.Errorf("Reply = %q", r.Reply)
	}
}
