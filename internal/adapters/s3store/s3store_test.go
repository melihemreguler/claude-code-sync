package s3store

import "testing"

// key/rel must round-trip a relative path through the configured prefix.
func TestKeyRelPrefix(t *testing.T) {
	s := &Store{prefix: "ccsync"}
	rel := "objects/abc/s.jsonl.age"
	if got := s.key(rel); got != "ccsync/objects/abc/s.jsonl.age" {
		t.Errorf("key(%q) = %q", rel, got)
	}
	if got := s.rel(s.key(rel)); got != rel {
		t.Errorf("rel(key(%q)) = %q, want round-trip", rel, got)
	}
}

func TestKeyRelEmptyPrefix(t *testing.T) {
	s := &Store{prefix: ""}
	rel := "manifest"
	if got := s.key(rel); got != rel {
		t.Errorf("key with empty prefix = %q, want %q", got, rel)
	}
	if got := s.rel(rel); got != rel {
		t.Errorf("rel with empty prefix = %q, want %q", got, rel)
	}
}
