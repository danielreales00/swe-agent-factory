// Package bridge is the Telegram ↔ pi protocol translator that puts pi in a
// Telegram chat. Increment 2.2 lands the Telegram client + allowlist; pi
// wiring follows in 2.4+.
package bridge

import (
	"fmt"
	"strconv"
	"strings"
)

// Allowlist is an immutable set of Telegram user IDs permitted to drive the
// bridge. Non-listed users are silently ignored — no echo, no error message.
type Allowlist struct {
	ids map[int64]struct{}
}

func NewAllowlist(ids []int64) *Allowlist {
	a := &Allowlist{ids: make(map[int64]struct{}, len(ids))}
	for _, id := range ids {
		a.ids[id] = struct{}{}
	}
	return a
}

func (a *Allowlist) Has(id int64) bool {
	if a == nil {
		return false
	}
	_, ok := a.ids[id]
	return ok
}

func (a *Allowlist) Size() int {
	if a == nil {
		return 0
	}
	return len(a.ids)
}

// IDs returns the set as a sorted-by-insertion slice. Useful for startup logs.
func (a *Allowlist) IDs() []int64 {
	if a == nil {
		return nil
	}
	out := make([]int64, 0, len(a.ids))
	for id := range a.ids {
		out = append(out, id)
	}
	return out
}

// ParseIDs parses a comma-separated list of int64 IDs. Empty entries (e.g.
// "1,,2") are skipped. Whitespace is trimmed. Returns an error if any entry
// is non-numeric.
func ParseIDs(s string) ([]int64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid user id %q: %w", p, err)
		}
		out = append(out, id)
	}
	return out, nil
}
