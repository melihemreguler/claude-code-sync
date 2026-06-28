// Package blobstore adapts an object/blob backend (S3, Google Drive, …) to the
// ports.Storage interface. A BlobStore exposes flat key→content access; Mirror
// keeps a local directory in sync with it so the rest of ccsync can treat any
// blob backend exactly like the git working copy.
package blobstore

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/melihemreguler/claude-code-sync/internal/fileutil"
)

// manifestRel is the well-known blob whose presence means "the chain has data".
const manifestRel = "manifest"

// BlobStore is a flat key→content store. Keys are forward-slash relative paths
// (e.g. "manifest", "objects/<hash>/<file>.age"); versions are content MD5 hex,
// which both S3 (ETag for single-part objects) and Drive (md5Checksum) provide.
type BlobStore interface {
	List(ctx context.Context) (map[string]string, error) // rel -> md5 hex
	Get(ctx context.Context, rel string) ([]byte, error)
	Put(ctx context.Context, rel string, data []byte) error
	Exists(ctx context.Context, rel string) (bool, error)
}

// Mirror keeps a local directory in sync with a BlobStore and satisfies
// ports.Storage.
//
// Concurrency note: unlike the git backend (which rebases on push rejection),
// blob backends write last-writer-wins per key and have no transactional update.
// Session objects are content-addressed so they never collide, but the single
// mutable "manifest" blob can be clobbered if two devices sync at the same
// instant — a device's just-recorded folder mapping or object metadata may be
// dropped (it self-heals on that device's next sync). Avoid simultaneous syncs;
// a proper lock / optimistic-concurrency update is planned (see ROADMAP P4).
type Mirror struct {
	blobs BlobStore
	dir   string
}

// NewMirror returns a Mirror backing dir with blobs.
func NewMirror(blobs BlobStore, dir string) *Mirror {
	return &Mirror{blobs: blobs, dir: dir}
}

// RootDir returns the local mirror directory.
func (m *Mirror) RootDir() string { return m.dir }

// EnsureLocal creates the mirror directory.
func (m *Mirror) EnsureLocal() error { return os.MkdirAll(m.dir, 0o755) }

// RemoteHasContent reports whether the backend already holds a manifest.
func (m *Mirror) RemoteHasContent() (bool, error) {
	return m.blobs.Exists(context.Background(), manifestRel)
}

// Pull downloads every remote blob whose content differs from the local mirror.
func (m *Mirror) Pull() error {
	ctx := context.Background()
	remote, err := m.blobs.List(ctx)
	if err != nil {
		return err
	}
	for rel, sum := range remote {
		local := filepath.Join(m.dir, filepath.FromSlash(rel))
		if fileMD5(local) == sum {
			continue
		}
		data, err := m.blobs.Get(ctx, rel)
		if err != nil {
			return err
		}
		if err := fileutil.WriteFileAtomic(local, data, time.Time{}); err != nil {
			return err
		}
	}
	return nil
}

// Push uploads every local mirror file whose content differs from the backend.
// The message argument is ignored (blob backends have no commit concept).
func (m *Mirror) Push(string) error {
	ctx := context.Background()
	remote, err := m.blobs.List(ctx)
	if err != nil {
		return err
	}
	return filepath.WalkDir(m.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), fileutil.TmpSuffix) {
			return nil
		}
		relOS, err := filepath.Rel(m.dir, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if remote[rel] == md5hex(data) {
			return nil
		}
		return m.blobs.Put(ctx, rel, data)
	})
}

func md5hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

func fileMD5(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return md5hex(data)
}
