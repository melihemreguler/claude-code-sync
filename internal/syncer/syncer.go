// Package syncer mirrors selected Claude Code project folders between the local
// machine and a git-backed data repo. Sync is additive (it never deletes session
// files) and last-writer-wins per file, with git union-merge of *.jsonl logs as
// the backstop for concurrent appends.
package syncer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/gitutil"
	"github.com/melihemreguler/claude-code-sync/internal/registry"
)

// tmpSuffix marks in-progress atomic writes; such files are never synced.
const tmpSuffix = ".ccsync.tmp"

// ProjectsSubdir is where Claude Code stores per-project session files, mirrored
// at the same path inside the data repo.
const ProjectsSubdir = "projects"

// Result tallies what a copy pass touched.
type Result struct {
	Files    int
	Projects int
}

// Syncer orchestrates pull/push for a single device configuration.
type Syncer struct {
	cfg *config.Config
}

// New returns a Syncer bound to cfg.
func New(cfg *config.Config) *Syncer {
	return &Syncer{cfg: cfg}
}

// EnsureRepo makes sure the data repo is cloned locally at cfg.WorkDir.
func (s *Syncer) EnsureRepo() error {
	if gitutil.IsRepo(s.cfg.WorkDir) {
		return nil
	}
	if s.cfg.RepoURL == "" {
		return fmt.Errorf("no repoUrl configured")
	}
	parent := filepath.Dir(s.cfg.WorkDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	fmt.Printf("Cloning %s …\n", s.cfg.RepoURL)
	return gitutil.Stream(parent, "clone", s.cfg.RepoURL, s.cfg.WorkDir)
}

// SeedRepo ensures the data repo has the structure ccsync expects: a projects/
// directory and a .gitattributes that union-merges JSONL session logs so
// concurrent appends from two devices auto-merge instead of conflicting.
func (s *Syncer) SeedRepo() error {
	if err := os.MkdirAll(filepath.Join(s.cfg.WorkDir, ProjectsSubdir), 0o755); err != nil {
		return err
	}
	keep := filepath.Join(s.cfg.WorkDir, ProjectsSubdir, ".gitkeep")
	if _, err := os.Stat(keep); os.IsNotExist(err) {
		if err := os.WriteFile(keep, []byte{}, 0o644); err != nil {
			return err
		}
	}
	attr := filepath.Join(s.cfg.WorkDir, ".gitattributes")
	if _, err := os.Stat(attr); os.IsNotExist(err) {
		content := "# Session logs are append-only — union-merge concurrent edits.\n*.jsonl merge=union\n"
		if err := os.WriteFile(attr, []byte(content), 0o644); err != nil {
			return err
		}
	}
	ignore := filepath.Join(s.cfg.WorkDir, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		if err := os.WriteFile(ignore, []byte("*"+tmpSuffix+"\n.DS_Store\n"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// RefreshRepo ensures the repo exists locally and is up to date with the remote,
// without touching the Claude directory. Used before mutating synced state (e.g.
// the device roster) so we don't act on or push from a stale clone.
func (s *Syncer) RefreshRepo() error {
	if err := s.EnsureRepo(); err != nil {
		return err
	}
	has, err := gitutil.RemoteHasBranches(s.cfg.WorkDir)
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return gitutil.Stream(s.cfg.WorkDir, "pull", "--rebase", "--autostash")
}

// Pull fetches remote changes and applies newer sessions into the Claude dir.
func (s *Syncer) Pull() (Result, error) {
	if err := s.EnsureRepo(); err != nil {
		return Result{}, err
	}
	hasBranches, err := gitutil.RemoteHasBranches(s.cfg.WorkDir)
	if err != nil {
		return Result{}, err
	}
	if !hasBranches {
		return Result{}, nil // fresh repo, nothing to pull
	}
	if err := gitutil.Stream(s.cfg.WorkDir, "pull", "--rebase", "--autostash"); err != nil {
		return Result{}, err
	}
	return copyNewer(s.cfg.WorkDir, s.cfg.ClaudeDir, s.cfg.Include, s.cfg.Exclude)
}

// Push copies newer local sessions into the repo, records this device, commits
// and pushes. It retries once after a rebase if the remote moved underneath us.
func (s *Syncer) Push() (Result, error) {
	if err := s.EnsureRepo(); err != nil {
		return Result{}, err
	}
	res, err := copyNewer(s.cfg.ClaudeDir, s.cfg.WorkDir, s.cfg.Include, s.cfg.Exclude)
	if err != nil {
		return res, err
	}

	reg, err := registry.Load(s.cfg.WorkDir)
	if err != nil {
		return res, err
	}
	reg.Upsert(s.cfg.Device, config.Platform())
	if err := reg.Save(s.cfg.WorkDir); err != nil {
		return res, err
	}

	changed, err := gitutil.HasChanges(s.cfg.WorkDir)
	if err != nil {
		return res, err
	}
	if !changed {
		return res, nil
	}

	if _, err := gitutil.Run(s.cfg.WorkDir, "add", "-A"); err != nil {
		return res, err
	}
	msg := fmt.Sprintf("sync: %d file(s) from %s", res.Files, s.cfg.Device)
	if _, err := gitutil.Run(s.cfg.WorkDir, "commit", "-m", msg); err != nil {
		return res, err
	}
	if err := s.doPush(); err != nil {
		if e := gitutil.Stream(s.cfg.WorkDir, "pull", "--rebase", "--autostash"); e != nil {
			return res, fmt.Errorf("push rejected and rebase failed: %w", e)
		}
		if err := s.doPush(); err != nil {
			return res, fmt.Errorf("push failed after rebase: %w", err)
		}
	}
	return res, nil
}

// Sync pulls remote changes, then pushes local ones.
func (s *Syncer) Sync() (in Result, out Result, err error) {
	if in, err = s.Pull(); err != nil {
		return in, out, err
	}
	out, err = s.Push()
	return in, out, err
}

// doPush pushes the current branch, setting upstream on the first push.
func (s *Syncer) doPush() error {
	if gitutil.HasUpstream(s.cfg.WorkDir) {
		return gitutil.Stream(s.cfg.WorkDir, "push")
	}
	return gitutil.Stream(s.cfg.WorkDir, "push", "-u", "origin", "HEAD")
}

// copyNewer mirrors matching project folders from src/projects to dst/projects,
// copying a file only when it is missing or strictly newer at the source. It
// never deletes anything at the destination.
func copyNewer(src, dst string, include, exclude []string) (Result, error) {
	var res Result
	srcProjects := filepath.Join(src, ProjectsSubdir)
	entries, err := os.ReadDir(srcProjects)
	if err != nil {
		if os.IsNotExist(err) {
			return res, nil
		}
		return res, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !MatchFilter(name, include, exclude) {
			continue
		}
		n, err := copyTreeNewer(filepath.Join(srcProjects, name), filepath.Join(dst, ProjectsSubdir, name))
		if err != nil {
			return res, err
		}
		if n > 0 {
			res.Projects++
			res.Files += n
		}
	}
	return res, nil
}

func copyTreeNewer(srcDir, dstDir string) (int, error) {
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
		if strings.HasSuffix(d.Name(), tmpSuffix) {
			return nil // never propagate our own in-progress writes
		}
		srcInfo, err := d.Info()
		if err != nil {
			return err
		}
		if dstInfo, err := os.Stat(target); err == nil {
			// Content-equal files are skipped regardless of mtime. git does not
			// preserve mtimes, so without this a checkout would look "newer" and
			// cause churn; comparing content avoids needless overwrites and
			// shrinks the window in which a stale copy could clobber edits.
			same, err := sameContent(path, target, srcInfo, dstInfo)
			if err != nil {
				return err
			}
			if same {
				return nil
			}
			if !srcInfo.ModTime().After(dstInfo.ModTime()) {
				return nil // different content but destination is at least as new
			}
		}
		if err := copyFile(path, target, srcInfo); err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}

// sameContent reports whether two files have identical bytes, short-circuiting
// on size before hashing.
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

// copyFile copies src to dst atomically (via a temp file + rename) and preserves
// the source modification time so newness comparisons stay meaningful.
func copyFile(src, dst string, srcInfo fs.FileInfo) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + tmpSuffix
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
