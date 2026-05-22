package axiom

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// stringField coerces an Axiom payload value (any) to a string suitable
// for clustering. JSON booleans/numbers/strings all become canonical
// string form; nil → "".
func stringField(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return strings.TrimRight(strings.TrimRight(
			fmt.Sprintf("%f", x), "0"), ".")
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(b)
	}
}

// extractTime reads a _time column value (raw JSON) and parses it as a
// timestamp. Accepts RFC3339, RFC3339Nano, or an epoch number.
func extractTime(raw json.RawMessage) time.Time {
	if len(raw) == 0 {
		return time.Time{}
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				return t
			}
		}
	}
	var n float64
	if err := json.Unmarshal(raw, &n); err == nil {
		// Axiom epochs are ms since unix.
		return time.UnixMilli(int64(n)).UTC()
	}
	return time.Time{}
}

// oneLine renders a payload map as a single-line JSON string for evidence.
func oneLine(p map[string]any) string {
	b, err := json.Marshal(p)
	if err != nil {
		return fmt.Sprintf("%v", p)
	}
	return string(b)
}
