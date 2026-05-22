package axiom

import (
	"encoding/json"
	"fmt"
)

// TabularToRows ports `audit.clj/tabular->rows`. The response packs values
// column-major: parallel `fields` (name+type) and `columns` (vec of vecs).
// Transpose into a slice of {field-name → raw-json-value} maps.
func TabularToRows(t Table) []map[string]json.RawMessage {
	if len(t.Columns) == 0 {
		return nil
	}
	nRows := len(t.Columns[0])
	rows := make([]map[string]json.RawMessage, 0, nRows)
	for r := 0; r < nRows; r++ {
		row := make(map[string]json.RawMessage, len(t.Fields))
		for i, f := range t.Fields {
			if i < len(t.Columns) && r < len(t.Columns[i]) {
				row[f.Name] = t.Columns[i][r]
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// FlattenAll runs TabularToRows over every table and returns one big slice.
func FlattenAll(tables []Table) []map[string]json.RawMessage {
	var all []map[string]json.RawMessage
	for _, t := range tables {
		all = append(all, TabularToRows(t)...)
	}
	return all
}

// UnpackPayload reads the nested `p` JSON the build-query projects.
// The bot's structured log is double-encoded: Axiom column `p` is itself
// a JSON object (sometimes a stringified one). Return a flat map keyed
// by string. `:event` is left as a JSON-encoded string for the caller
// to coerce. (Port of `audit.clj/normalize-event`.)
func UnpackPayload(row map[string]json.RawMessage) (map[string]any, error) {
	pRaw, ok := row["p"]
	if !ok {
		// Older shape: row is already flat.
		flat := make(map[string]any, len(row))
		for k, v := range row {
			var x any
			if err := json.Unmarshal(v, &x); err == nil {
				flat[k] = x
			}
		}
		return flat, nil
	}
	var p map[string]any
	if err := json.Unmarshal(pRaw, &p); err == nil {
		return p, nil
	}
	// `p` may have been double-encoded as a JSON *string*.
	var s string
	if err := json.Unmarshal(pRaw, &s); err == nil {
		if err := json.Unmarshal([]byte(s), &p); err == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("could not decode payload column p")
}
