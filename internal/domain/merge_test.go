package domain

import "testing"

func TestManifestMerge(t *testing.T) {
	a := NewManifest()
	a.UpsertDevice("A", "darwin", nil, nil)
	a.Devices[0].LastSync = "2026-01-01T00:00:00Z"
	a.RecordProject("github.com/x/p", "p", "A", "-A-p")
	a.SetObject("github.com/x/p", "s.jsonl", ObjectMeta{Hash: "h1", MTime: 100})

	b := NewManifest()
	b.UpsertDevice("B", "linux", nil, nil)
	b.RecordProject("github.com/x/p", "p", "B", "-B-p")
	b.SetObject("github.com/x/p", "s.jsonl", ObjectMeta{Hash: "h2", MTime: 200}) // newer
	b.RecordProject("github.com/x/q", "q", "B", "-B-q")

	a.Merge(b)

	if len(a.Devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(a.Devices))
	}
	p := a.Projects["github.com/x/p"]
	if p.Folders["A"] != "-A-p" || p.Folders["B"] != "-B-p" {
		t.Errorf("folder mappings not unioned: %v", p.Folders)
	}
	if got := p.Objects["s.jsonl"]; got.MTime != 200 || got.Hash != "h2" {
		t.Errorf("object merge should keep the newer mtime, got %+v", got)
	}
	if _, ok := a.Projects["github.com/x/q"]; !ok {
		t.Error("project only in b was dropped")
	}
}

func TestManifestMergeDeviceLatestWins(t *testing.T) {
	a := NewManifest()
	a.UpsertDevice("A", "darwin", []string{"old"}, nil)
	a.Devices[0].LastSync = "2026-01-01T00:00:00Z"

	b := NewManifest()
	b.UpsertDevice("A", "darwin", []string{"new"}, nil)
	b.Devices[0].LastSync = "2026-06-01T00:00:00Z"

	a.Merge(b)
	if len(a.Devices) != 1 {
		t.Fatalf("same device must not duplicate, got %d", len(a.Devices))
	}
	if a.Devices[0].Include[0] != "new" {
		t.Errorf("latest LastSync should win, got %v", a.Devices[0].Include)
	}
}
