package axiom

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiURL = "https://api.axiom.co/v1/datasets/_apl?format=tabular"

// Field mirrors Axiom's tabular response: each :field is {name, type}.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// Table is one element under "tables". `columns` is column-major: a vec of
// vecs where columns[i] is the i-th column's values in row order.
// (Port of `audit.clj/tabular->rows`.)
type Table struct {
	Fields  []Field           `json:"fields"`
	Columns [][]json.RawMessage `json:"columns"`
}

// APLResponse mirrors what `audit.clj/post-apl` reads — only the fields
// we care about. `matches` is the older flat shape; `tables` is the
// current tabular shape that build-query targets.
type APLResponse struct {
	Tables  []Table           `json:"tables"`
	Matches []json.RawMessage `json:"matches"`
}

type Client struct {
	Token      string
	HTTPClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		Token: token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PostAPL ports `audit.clj/post-apl`. Bearer-auths, POSTs the APL string +
// window, returns the parsed response. Returns an error on non-2xx.
func (c *Client) PostAPL(ctx context.Context, apl string, start, end time.Time) (*APLResponse, error) {
	body, err := json.Marshal(map[string]string{
		"apl":       apl,
		"startTime": start.UTC().Format(time.RFC3339Nano),
		"endTime":   end.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("axiom apl request: %w", err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("axiom apl http %d: %s", resp.StatusCode, string(rb))
	}
	var out APLResponse
	if err := json.Unmarshal(rb, &out); err != nil {
		return nil, fmt.Errorf("decode apl response: %w", err)
	}
	return &out, nil
}
