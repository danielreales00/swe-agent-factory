package state

import "github.com/danielreales00/swe-agent-factory/internal/workitem"

// Store is the dedup/audit boundary. Phase 1 backs this with JSON files;
// later phases may swap to sqlite without changing callers.
type Store interface {
	// Get returns the existing record for an ID, or (nil, nil) if unknown.
	Get(target, id string) (*workitem.WorkItem, error)
	// Put writes a WorkItem (idempotent — last write wins).
	Put(item workitem.WorkItem) error
	// List returns all known items for a target.
	List(target string) ([]workitem.WorkItem, error)
}
