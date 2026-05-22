package workitem

import (
	"crypto/sha1"
	"fmt"
	"strings"
)

// NewID builds a stable, filesystem-safe ID of the form "source-kind-key".
// The slashes in "axiom://capability-gaps/:create-account" make awful
// filenames, so we collapse to dashes after preserving the semantic form
// as Metadata["uri"].
func NewID(source SourceID, kind, key string) (id, uri string) {
	uri = fmt.Sprintf("%s://%s/%s", source, kind, key)
	id = sanitize(string(source)) + "-" + sanitize(kind) + "-" + sanitize(key)
	return id, uri
}

// InlineID hashes title+timestamp because inline tasks have no stable key.
func InlineID(title string, ts int64) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d|%s", ts, title)))
	return fmt.Sprintf("inline-%x", h[:6])
}

func sanitize(s string) string {
	s = strings.TrimPrefix(s, ":")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, ":", "_")
	return s
}
