package bridge

import (
	"context"
	"errors"
	"testing"
)

func TestParseShipRequestArgs(t *testing.T) {
	good := map[string]any{
		"branch": "swe-agent/format-money-negative-test",
		"title":  "Add negative test for format-money",
		"body":   "## Summary\n...\n## Decisions I made\n...",
	}
	req, ok := ParseShipRequestArgs(good)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	if req.Branch != good["branch"] || req.Title != good["title"] || req.Body != good["body"] {
		t.Errorf("got = %+v", req)
	}
}

func TestParseShipRequestArgs_Missing(t *testing.T) {
	cases := []map[string]any{
		nil,
		{},
		{"branch": "swe-agent/x"}, // missing title, body
		{"branch": "swe-agent/x", "title": "t"},
		{"title": "t", "body": "b"}, // missing branch
		{"branch": "", "title": "t", "body": "b"},
		{"branch": "swe-agent/x", "title": "", "body": "b"},
		{"branch": "swe-agent/x", "title": "t", "body": ""},
		{"branch": 42, "title": "t", "body": "b"}, // wrong type
	}
	for i, args := range cases {
		if _, ok := ParseShipRequestArgs(args); ok {
			t.Errorf("case %d: ok = true, want false (args=%v)", i, args)
		}
	}
}

func TestDefaultShipper_Ship_RoundtripWithStubs(t *testing.T) {
	var pushedBranch, pushedDir string
	var prReq ShipRequest
	var prDir string

	s := &DefaultShipper{
		Push: func(_ context.Context, worktree, branch string) error {
			pushedDir = worktree
			pushedBranch = branch
			return nil
		},
		CreatePR: func(_ context.Context, worktree string, req ShipRequest) (string, error) {
			prDir = worktree
			prReq = req
			return "https://github.com/owner/repo/pull/42", nil
		},
	}

	req := ShipRequest{Branch: "swe-agent/x", Title: "t", Body: "b"}
	url, err := s.Ship(context.Background(), "/wt", req)
	if err != nil {
		t.Fatalf("Ship err = %v", err)
	}
	if url != "https://github.com/owner/repo/pull/42" {
		t.Errorf("url = %q", url)
	}
	if pushedDir != "/wt" || pushedBranch != "swe-agent/x" {
		t.Errorf("push args: dir=%q branch=%q", pushedDir, pushedBranch)
	}
	if prDir != "/wt" || prReq != req {
		t.Errorf("createPR args: dir=%q req=%+v", prDir, prReq)
	}
}

func TestDefaultShipper_Ship_StopsOnPushFailure(t *testing.T) {
	prCalled := false
	s := &DefaultShipper{
		Push: func(_ context.Context, _, _ string) error {
			return errors.New("auth denied")
		},
		CreatePR: func(_ context.Context, _ string, _ ShipRequest) (string, error) {
			prCalled = true
			return "", nil
		},
	}
	_, err := s.Ship(context.Background(), "/wt", ShipRequest{Branch: "swe-agent/x", Title: "t", Body: "b"})
	if err == nil {
		t.Fatal("err = nil, want push failure")
	}
	if prCalled {
		t.Error("CreatePR called after push failure")
	}
}

func TestFirstHTTPSLine(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://github.com/owner/repo/pull/1\n", "https://github.com/owner/repo/pull/1"},
		{"Creating draft pull request...\nhttps://github.com/o/r/pull/2\n", "https://github.com/o/r/pull/2"},
		{"  https://x/y/pull/3  \n", "https://x/y/pull/3"},
		{"no url here", "no url here"},
	}
	for _, c := range cases {
		if got := firstHTTPSLine(c.in); got != c.want {
			t.Errorf("firstHTTPSLine(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
