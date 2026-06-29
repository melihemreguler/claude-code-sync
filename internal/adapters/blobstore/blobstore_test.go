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

func (m *memBlobs) Delete(_ context.Context, rel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, rel)
	return nil
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

// Delete must remove the blob from the backend so a dropped device shard does
// not reappear on another device's next pull.
func TestMirrorDeletePropagates(t *testing.T) {
	blobs := newMem()
	dirA := t.TempDir()
	a := NewMirror(blobs, dirA)
	if err := a.EnsureLocal(); err != nil {
		t.Fatal(err)
	}
	writeMirror(t, dirA, "manifests/A.age", "shardA")
	writeMirror(t, dirA, "manifests/B.age", "shardB")
	if err := a.Push(""); err != nil {
		t.Fatal(err)
	}

	existed, err := a.Delete("manifests/B.age")
	if err != nil || !existed {
		t.Fatalf("Delete: existed=%v err=%v", existed, err)
	}
	if ok, _ := blobs.Exists(context.Background(), "manifests/B.age"); ok {
		t.Fatal("B shard should be gone from the backend")
	}
	if _, err := os.Stat(filepath.Join(dirA, "manifests", "B.age")); !os.IsNotExist(err) {
		t.Fatal("B shard should be gone from the local mirror")
	}

	// A second device must not receive the deleted shard on pull.
	dirC := t.TempDir()
	c := NewMirror(blobs, dirC)
	if err := c.EnsureLocal(); err != nil {
		t.Fatal(err)
	}
	if err := c.Pull(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dirC, "manifests", "B.age")); !os.IsNotExist(err) {
		t.Fatal("deleted shard must not appear on another device's pull")
	}
	if _, err := os.Stat(filepath.Join(dirC, "manifests", "A.age")); err != nil {
		t.Fatalf("surviving shard should still pull: %v", err)
	}

	// Deleting an absent key reports not-existed without error.
	existed, err = a.Delete("manifests/missing.age")
	if err != nil || existed {
		t.Fatalf("deleting absent key: existed=%v err=%v", existed, err)
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
