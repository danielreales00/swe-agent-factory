package inline

import (
	"context"
	"strings"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/signal"
	"github.com/danielreales00/swe-agent-factory/internal/workitem"
)

// Source emits one inline_task WorkItem from a CLI --task flag. It bypasses
// all manifest configuration: the operator types the task on the command
// line, we wrap it in a WorkItem, downstream pipeline doesn't care.
type Source struct {
	target string
	task   string
}

func New(target, task string) *Source {
	return &Source{target: target, task: strings.TrimSpace(task)}
}

func (s *Source) Name() string { return "inline" }
func (s *Source) Kind() string { return "inline" }

func (s *Source) Collect(ctx context.Context, _ signal.Window) ([]workitem.WorkItem, error) {
	if s.task == "" {
		return nil, nil
	}
	now := time.Now().UTC()
	id := workitem.InlineID(s.task, now.UnixNano())
	title := firstLine(s.task)
	return []workitem.WorkItem{{
		ID:          id,
		Type:        workitem.TypeInlineTask,
		Source:      workitem.SourceInline,
		Target:      s.target,
		Title:       title,
		Description: s.task,
		Evidence: []workitem.Evidence{{
			Kind: "user_quote",
			Body: s.task,
		}},
		Metadata:  map[string]string{"uri": "inline://" + id},
		CreatedAt: now,
		Status:    workitem.StatusOpen,
	}}, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i > 0 {
		return s[:i]
	}
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
