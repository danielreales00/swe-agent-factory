package bridge

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// ShipRequest is the parsed payload of a `request_ship` tool call.
// The extension validates these fields server-side; the bridge re-checks
// only that the strings are non-empty before invoking git.
type ShipRequest struct {
	Branch string
	Title  string
	Body   string
}

// ParseShipRequestArgs pulls (branch, title, body) out of a tool_execution_start
// args map. Returns ok=false if any required field is missing or non-string.
func ParseShipRequestArgs(args map[string]any) (ShipRequest, bool) {
	branch, ok1 := args["branch"].(string)
	title, ok2 := args["title"].(string)
	body, ok3 := args["body"].(string)
	if !ok1 || !ok2 || !ok3 {
		return ShipRequest{}, false
	}
	if branch == "" || title == "" || body == "" {
		return ShipRequest{}, false
	}
	return ShipRequest{Branch: branch, Title: title, Body: body}, true
}

// Shipper pushes a branch and opens a draft PR. Implementations run in
// the bridge's goroutines, so they must be safe to call concurrently for
// different worktrees.
type Shipper interface {
	Ship(ctx context.Context, worktree string, req ShipRequest) (prURL string, err error)
}

// DefaultShipper shells out to `git push` and `gh pr create --draft`.
// Push and CreatePR are exposed for tests to override.
type DefaultShipper struct {
	Push     func(ctx context.Context, worktree, branch string) error
	CreatePR func(ctx context.Context, worktree string, req ShipRequest) (string, error)
}

func NewDefaultShipper() *DefaultShipper {
	return &DefaultShipper{
		Push:     pushBranch,
		CreatePR: createDraftPR,
	}
}

func (s *DefaultShipper) Ship(ctx context.Context, worktree string, req ShipRequest) (string, error) {
	if err := s.Push(ctx, worktree, req.Branch); err != nil {
		return "", err
	}
	return s.CreatePR(ctx, worktree, req)
}

func pushBranch(ctx context.Context, worktree, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "push", "--set-upstream", "origin", branch)
	cmd.Dir = worktree
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git push %s: %w (%s)",
			branch, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func createDraftPR(ctx context.Context, worktree string, req ShipRequest) (string, error) {
	cmd := exec.CommandContext(ctx, "gh", "pr", "create",
		"--draft",
		"--head", req.Branch,
		"--title", req.Title,
		"--body", req.Body,
	)
	cmd.Dir = worktree
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh pr create: %w (%s)",
			err, strings.TrimSpace(stderr.String()))
	}
	return firstHTTPSLine(stdout.String()), nil
}

// firstHTTPSLine returns the first line of out that begins with https://.
// `gh pr create` prints the PR URL on its own line; capturing only that
// skips any leading "Creating draft pull request..." noise.
func firstHTTPSLine(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "https://") {
			return line
		}
	}
	return strings.TrimSpace(out)
}
