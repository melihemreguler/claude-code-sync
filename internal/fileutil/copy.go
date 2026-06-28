// Package fileutil provides additive, content-aware file tree copying. A file is
// copied only when it is missing or differs at the destination; copies are
// atomic and never delete anything at the destination.
package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// TmpSuffix marks in-progress atomic writes; such files are never propagated.
const TmpSuffix = ".ccsync.tmp"

// CopyTree mirrors srcDir into dstDir, returning the number of files written.
// Content-equal files are skipped regardless of modification time (git does not
// preserve mtimes, so a checkout would otherwise look "newer"); for differing
// files, the source must be strictly newer to overwrite.
func CopyTree(srcDir, dstDir string) (int, error) {
	count := 0
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if strings.HasSuffix(d.Name(), TmpSuffix) {
			return nil
		}
		srcInfo, err := d.Info()
		if err != nil {
			return err
		}
		if dstInfo, err := os.Stat(target); err == nil {
			same, err := sameContent(path, target, srcInfo, dstInfo)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				return nil
			}
		}
		if err := copyFile(path, target, srcInfo); err != nil {
			return err
		}
		count++
		return nil
	})
	if os.IsNotExist(err) {
		return count, nil
	}
	return count, err
}

func sameContent(a, b string, ai, bi fs.FileInfo) (bool, error) {
	if ai.Size() != bi.Size() {
		return false, nil
	}
	ha, err := hashFile(a)
	if err != nil {
		return false, err
	}
	hb, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, srcInfo fs.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + TmpSuffix
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime())
	return nil
}
