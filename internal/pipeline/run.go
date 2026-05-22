package pipeline

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/signal"
	"github.com/danielreales00/swe-agent-factory/internal/state"
	"github.com/danielreales00/swe-agent-factory/internal/workitem"
)

// ScanInput is what `swe-agent scan` hands the pipeline.
type ScanInput struct {
	Target  string
	Sources []signal.Source
	Window  signal.Window
	Store   state.Store
	Out     io.Writer
}

// ScanResult is what we print + return for tests / programmatic callers.
type ScanResult struct {
	Items    []workitem.WorkItem
	Existing int
	New      int
}

// Scan runs every Source, dedups against the Store, persists new items,
// and prints a digest grouped by source kind.
func Scan(ctx context.Context, in ScanInput) (*ScanResult, error) {
	res := &ScanResult{}
	if in.Out == nil {
		return nil, fmt.Errorf("scan: Out writer is nil")
	}
	for _, src := range in.Sources {
		items, err := src.Collect(ctx, in.Window)
		if err != nil {
			fmt.Fprintf(in.Out, "  ✗ %s (%s): %v\n", src.Name(), src.Kind(), err)
			continue
		}
		for _, item := range items {
			existing, err := in.Store.Get(in.Target, item.ID)
			if err != nil {
				return nil, err
			}
			if existing != nil {
				// Refresh evidence + metadata but preserve lifecycle fields.
				item.Status = existing.Status
				item.PRUrl = existing.PRUrl
				res.Existing++
			} else {
				res.New++
			}
			if err := in.Store.Put(item); err != nil {
				return nil, err
			}
			res.Items = append(res.Items, item)
		}
	}
	renderDigest(in.Out, in.Target, in.Window, res)
	return res, nil
}

func renderDigest(w io.Writer, target string, window signal.Window, res *ScanResult) {
	fmt.Fprintln(w, separator)
	fmt.Fprintf(w, "scan  target=%s  window=%s → %s\n",
		target, fmtTime(window.Since), fmtTime(window.Until))
	fmt.Fprintf(w, "      items=%d  new=%d  known=%d\n",
		len(res.Items), res.New, res.Existing)
	fmt.Fprintln(w, separator)

	bySource := map[workitem.SourceID][]workitem.WorkItem{}
	for _, it := range res.Items {
		bySource[it.Source] = append(bySource[it.Source], it)
	}
	for src, items := range bySource {
		fmt.Fprintf(w, "\n# %s (%d)\n", src, len(items))
		for _, it := range items {
			marker := "·"
			if it.Status == workitem.StatusShipped {
				marker = "✓"
			}
			fmt.Fprintf(w, "  %s [%s] %s\n", marker, it.Type, it.Title)
			fmt.Fprintf(w, "      id=%s  evidence=%d\n", it.ID, len(it.Evidence))
			if desc := strings.TrimSpace(it.Description); desc != "" {
				for _, line := range strings.Split(desc, "\n") {
					if line == "" {
						continue
					}
					fmt.Fprintf(w, "      %s\n", line)
				}
			}
		}
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(w, "  (no items)")
	}
}

const separator = "──────────────────────────────────────────────────────────────────────"

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.UTC().Format(time.RFC3339)
}
