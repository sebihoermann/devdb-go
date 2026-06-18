package storage

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// NewID returns a 32-character lowercase hex identifier.
func NewID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// ResolveID matches a full id or unique prefix against candidates.
func ResolveID(prefix string, candidates []string) (string, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return "", fmt.Errorf("empty id prefix")
	}
	var matches []string
	for _, c := range candidates {
		id := strings.ToLower(c)
		if id == prefix || strings.HasPrefix(id, prefix) {
			matches = append(matches, c)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no match for id prefix %q", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous id prefix %q (%d matches)", prefix, len(matches))
	}
}
