// Package blobstoretest provides a reusable contract test for
// blobstore.BlobStore implementations, so the in-memory reference, the S3 store
// and the Google Drive store are all held to the same Put/Get/List/Exists/Delete
// semantics. The real backends can only be exercised against live credentials,
// so their integration tests call Run when those are configured; this package's
// own test runs Run against the in-memory reference on every CI build.
package blobstoretest

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"sync"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/blobstore"
)

// Run exercises the BlobStore contract against store. It writes a couple of test
// keys under "objects/contract/" and cleans them up; point it at a store whose
// namespace is safe to write into.
func Run(t *testing.T, store blobstore.BlobStore) {
	t.Helper()
	ctx := context.Background()
	rel := "objects/contract/sample.age"
	body := []byte("ciphertext-payload")

	if ok, err := store.Exists(ctx, rel); err != nil || ok {
		t.Fatalf("Exists before Put = %v (err %v), want false", ok, err)
	}

	if err := store.Put(ctx, rel, body); err != nil {
		t.Fatalf("Put: %v", err)
	}
	t.Cleanup(func() { _ = store.Delete(ctx, rel) })

	if ok, err := store.Exists(ctx, rel); err != nil || !ok {
		t.Fatalf("Exists after Put = %v (err %v), want true", ok, err)
	}
	got, err := store.Get(ctx, rel)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("Get = %q (err %v), want %q", got, err, body)
	}

	list, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if v, ok := list[rel]; !ok || v == "" {
		t.Fatalf("List should contain %q with a non-empty content version, got %v", rel, list)
	}

	if err := store.Delete(ctx, rel); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if ok, err := store.Exists(ctx, rel); err != nil || ok {
		t.Fatalf("Exists after Delete = %v (err %v), want false", ok, err)
	}
	if err := store.Delete(ctx, rel); err != nil {
		t.Errorf("Delete of an absent key should be a no-op, got %v", err)
	}
}

// Mem is an in-memory blobstore.BlobStore — the reference implementation the
// contract is validated against, and a convenient fake for other tests.
type Mem struct {
	mu   sync.Mutex
	data map[string][]byte
}

// NewMem returns an empty in-memory store.
func NewMem() *Mem { return &Mem{data: map[string][]byte{}} }

func (m *Mem) List(context.Context) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for k, v := range m.data {
		sum := md5.Sum(v)
		out[k] = hex.EncodeToString(sum[:])
	}
	return out, nil
}

func (m *Mem) Get(_ context.Context, rel string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]byte(nil), m.data[rel]...), nil
}

func (m *Mem) Put(_ context.Context, rel string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[rel] = append([]byte(nil), data...)
	return nil
}

func (m *Mem) Exists(_ context.Context, rel string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.data[rel]
	return ok, nil
}

func (m *Mem) Delete(_ context.Context, rel string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, rel)
	return nil
}
