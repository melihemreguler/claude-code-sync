package fileutil

import (
	"path/filepath"
	"testing"
)

func TestSafeJoin(t *testing.T) {
	base := "/data/objects/abc"
	ok := map[string]string{
		"s.jsonl":         filepath.Join(base, "s.jsonl"),
		"memory/notes.md": filepath.Join(base, "memory/notes.md"),
		"./s.jsonl":       filepath.Join(base, "s.jsonl"),
	}
	for rel, want := range ok {
		got, err := SafeJoin(base, rel)
		if err != nil || got != want {
			t.Errorf("SafeJoin(%q) = %q,%v; want %q", rel, got, err, want)
		}
	}
	// Note: a leading "/" is cleaned by Join into base, so it stays safe; only
	// traversal that actually climbs out of base is rejected.
	for _, bad := range []string{"../evil", "../../etc/passwd", "a/../../escape"} {
		if _, err := SafeJoin(base, bad); err == nil {
			t.Errorf("SafeJoin(%q) should have rejected the path", bad)
		}
	}
}
