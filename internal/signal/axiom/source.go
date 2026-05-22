package axiom

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danielreales00/swe-agent-factory/internal/config"
	"github.com/danielreales00/swe-agent-factory/internal/signal"
	"github.com/danielreales00/swe-agent-factory/internal/workitem"
)

// Source is the manifest-driven Axiom adapter.
//
// One Source corresponds to one `signals[]` entry of kind: axiom. It runs
// the manifest's APL string against the given dataset, clusters the result
// by `cluster_by` (typically the `action` keyword from a :agent-action
// error stream), and emits one WorkItem per cluster.
type Source struct {
	signal config.Signal
	target string
	client *Client
}

// MaxEvidenceSamples caps how many log samples we attach per WorkItem so
// the JSON state files don't explode for high-frequency clusters.
const MaxEvidenceSamples = 5

func New(target string, s config.Signal, c *Client) *Source {
	return &Source{signal: s, target: target, client: c}
}

func (s *Source) Name() string {
	if s.signal.Name != "" {
		return s.signal.Name
	}
	return s.signal.Kind
}

func (s *Source) Kind() string { return s.signal.Kind }

type cluster struct {
	key      string
	samples  []map[string]any
	earliest time.Time
	latest   time.Time
}

func (s *Source) Collect(ctx context.Context, w signal.Window) ([]workitem.WorkItem, error) {
	if s.signal.Query == "" {
		return nil, fmt.Errorf("axiom source %q has no query", s.Name())
	}
	resp, err := s.client.PostAPL(ctx, s.signal.Query, w.Since, w.Until)
	if err != nil {
		return nil, err
	}
	rows := FlattenAll(resp.Tables)
	if len(rows) == 0 {
		return nil, nil
	}

	clusterKey := s.signal.ClusterBy
	if clusterKey == "" {
		clusterKey = "action"
	}

	groups := map[string]*cluster{}
	for _, row := range rows {
		payload, err := UnpackPayload(row)
		if err != nil {
			continue
		}
		k := stringField(payload[clusterKey])
		if k == "" {
			k = "unknown"
		}
		c, ok := groups[k]
		if !ok {
			c = &cluster{key: k}
			groups[k] = c
		}
		if len(c.samples) < MaxEvidenceSamples {
			c.samples = append(c.samples, payload)
		}
		if ts := extractTime(row["_time"]); !ts.IsZero() {
			if c.earliest.IsZero() || ts.Before(c.earliest) {
				c.earliest = ts
			}
			if ts.After(c.latest) {
				c.latest = ts
			}
		}
	}

	out := make([]workitem.WorkItem, 0, len(groups))
	for _, c := range groups {
		id, uri := workitem.NewID(workitem.SourceAxiom, s.Name(), c.key)
		ev := make([]workitem.Evidence, 0, len(c.samples))
		for _, p := range c.samples {
			ev = append(ev, workitem.Evidence{
				Kind: "log_sample",
				Body: oneLine(p),
			})
		}
		out = append(out, workitem.WorkItem{
			ID:          id,
			Type:        s.triageType(),
			Source:      workitem.SourceAxiom,
			Target:      s.target,
			Title:       fmt.Sprintf("capability gap: %s", c.key),
			Description: describeCluster(c, clusterKey),
			Evidence:    ev,
			Metadata: map[string]string{
				"uri":         uri,
				"signal_name": s.Name(),
				"dataset":     s.signal.Dataset,
				"cluster_key": c.key,
				"count":       fmt.Sprintf("%d", len(c.samples)),
			},
			CreatedAt: time.Now().UTC(),
			Status:    workitem.StatusOpen,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *Source) triageType() workitem.Type {
	switch s.signal.TriageAs {
	case "feature_request":
		return workitem.TypeFeatureRequest
	case "bug_report":
		return workitem.TypeBugReport
	case "", "capability_gap":
		return workitem.TypeCapabilityGap
	default:
		return workitem.TypeCapabilityGap
	}
}

func describeCluster(c *cluster, clusterKey string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Cluster %s = %q\n", clusterKey, c.key)
	fmt.Fprintf(&sb, "Samples retained: %d (cap %d)\n", len(c.samples), MaxEvidenceSamples)
	if !c.earliest.IsZero() {
		fmt.Fprintf(&sb, "Window: %s → %s\n",
			c.earliest.Format(time.RFC3339),
			c.latest.Format(time.RFC3339))
	}
	if len(c.samples) > 0 {
		if msg := stringField(c.samples[0]["error.message"]); msg != "" {
			fmt.Fprintf(&sb, "First error.message: %s\n", msg)
		}
	}
	return sb.String()
}
