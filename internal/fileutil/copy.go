// Package fileutil provides atomic file writes and content hashing used by the
// sync engine.
package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TmpSuffix marks in-progress atomic writes; such files are never propagated.
const TmpSuffix = ".ccsync.tmp"

// SafeJoin joins a slash-separated relative path onto base, returning an error if
// the result would escape base (e.g. via "../"). Guards against a corrupt or
// hostile manifest writing outside the intended directory.
func SafeJoin(base, rel string) (string, error) {
	p := filepath.Join(base, filepath.FromSlash(rel))
	within, err := filepath.Rel(base, p)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe path %q escapes %q", rel, base)
	}
	return p, nil
}

// HashBytes returns the hex-encoded sha256 of data.
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// WriteFileAtomic writes data to path via a temp file + rename, creating parent
// directories. If mtime is non-zero it is applied to the written file so that
// newness comparisons remain meaningful across machines.
func WriteFileAtomic(path string, data []byte, mtime time.Time) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + TmpSuffix
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if !mtime.IsZero() {
		_ = os.Chtimes(path, mtime, mtime)
	}
	return nil
}
