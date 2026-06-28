package blobstore

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// memBlobs is an in-memory BlobStore for testing the mirror logic without a real
// cloud backend.
type memBlobs struct {
	mu   sync.Mutex
	data map[string][]byte
	puts int
}

func newMem() *memBlobs { return &memBlobs{data: map[string][]byte{}} }

func (m *memBlobs) List(context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for k, v := range m.data {
		out[k] = md5hex(v)
	}
	return out, nil
}

func (m *memBlobs) Get(_ context.Context, rel string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data[rel]...), nil
}

func (m *memBlobs) Put(_ context.Context, rel string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[rel] = append([]byte(nil), data...)
	m.puts++
	return nil
}

func (m *memBlobs) Exists(_ context.Context, rel string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[rel]
	return ok, nil
}

// Two mirrors over one backend behave like two devices sharing a bucket.
func TestMirrorRoundTripBetweenDevices(t *testing.T) {
	blobs := newMem()

	dirA := t.TempDir()
	a := NewMirror(blobs, dirA)
	if err := a.EnsureLocal(); err != nil {
		t.Fatal(err)
	}
	writeMirror(t, dirA, "manifest", "m1")
	writeMirror(t, dirA, "objects/abc/s.age", "ciphertext")
	if err := a.Push(""); err != nil {
		t.Fatal(err)
	}

	dirB := t.TempDir()
	b := NewMirror(blobs, dirB)
	if err := b.EnsureLocal(); err != nil {
		t.Fatal(err)
	}
	if has, _ := b.RemoteHasContent(); !has {
		t.Fatal("RemoteHasContent should be true after A pushed a manifest")
	}
	if err := b.Pull(); err != nil {
		t.Fatal(err)
	}
	if got := readMirror(t, dirB, "objects/abc/s.age"); got != "ciphertext" {
		t.Fatalf("B did not receive object: %q", got)
	}
}

// Push must skip blobs that are already up to date.
func TestMirrorPushSkipsUnchanged(t *testing.T) {
	blobs := newMem()
	dir := t.TempDir()
	m := NewMirror(blobs, dir)
	_ = m.EnsureLocal()
	writeMirror(t, dir, "manifest", "m1")
	if err := m.Push(""); err != nil {
		t.Fatal(err)
	}
	before := blobs.puts
	if err := m.Push(""); err != nil { // nothing changed
		t.Fatal(err)
	}
	if blobs.puts != before {
		t.Fatalf("expected no new puts, got %d extra", blobs.puts-before)
	}
}

func writeMirror(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readMirror(t *testing.T, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
