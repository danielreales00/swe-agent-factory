package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/danielreales00/swe-agent-factory/internal/workitem"
)

// FileStore writes one JSON file per WorkItem under:
//
//	<root>/<target>/workitems/<id>.json
//
// Dedup is by file existence; status field tracks lifecycle (open →
// shipped/closed). At low scale these double as a free git audit log.
type FileStore struct {
	Root string
}

func NewFileStore(root string) *FileStore {
	return &FileStore{Root: root}
}

func (s *FileStore) dir(target string) string {
	return filepath.Join(s.Root, target, "workitems")
}

func (s *FileStore) path(target, id string) string {
	return filepath.Join(s.dir(target), id+".json")
}

func (s *FileStore) Get(target, id string) (*workitem.WorkItem, error) {
	b, err := os.ReadFile(s.path(target, id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var w workitem.WorkItem
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, fmt.Errorf("decode %s: %w", s.path(target, id), err)
	}
	return &w, nil
}

func (s *FileStore) Put(item workitem.WorkItem) error {
	if item.Target == "" || item.ID == "" {
		return fmt.Errorf("filestore: WorkItem missing target or id")
	}
	if err := os.MkdirAll(s.dir(item.Target), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(item.Target, item.ID), b, 0o644)
}

func (s *FileStore) List(target string) ([]workitem.WorkItem, error) {
	entries, err := os.ReadDir(s.dir(target))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]workitem.WorkItem, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.dir(target), e.Name()))
		if err != nil {
			return nil, err
		}
		var w workitem.WorkItem
		if err := json.Unmarshal(b, &w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, nil
}
