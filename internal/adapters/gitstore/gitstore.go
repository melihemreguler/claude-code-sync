// Package gitstore implements ports.Storage on top of a git repository, shelling
// out to the user's git so existing credentials and SSH config are reused.
package gitstore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
	"github.com/melihemreguler/claude-code-sync/internal/fileutil"
	"github.com/melihemreguler/claude-code-sync/internal/gitutil"
)

// Store is a git-backed storage backend with a local working clone.
type Store struct {
	repoURL string
	workDir string
}

// New returns a git Store that clones repoURL into workDir on demand.
func New(repoURL, workDir string) *Store {
	return &Store{repoURL: repoURL, workDir: workDir}
}

// RootDir returns the local working directory.
func (s *Store) RootDir() string { return s.workDir }

// EnsureLocal clones the repo into workDir if needed and ensures git hygiene.
func (s *Store) EnsureLocal() error {
	if gitutil.IsRepo(s.workDir) {
		if err := s.checkOrigin(); err != nil {
			return err
		}
	} else {
		if s.repoURL == "" {
			return fmt.Errorf("no repository URL configured")
		}
		parent := filepath.Dir(s.workDir)
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
		fmt.Printf("Cloning %s …\n", s.repoURL)
		if err := gitutil.Stream(parent, "clone", s.repoURL, s.workDir); err != nil {
			return err
		}
	}
	ignore := filepath.Join(s.workDir, ".gitignore")
	if _, err := os.Stat(ignore); os.IsNotExist(err) {
		return os.WriteFile(ignore, []byte("*"+fileutil.TmpSuffix+"\n.DS_Store\n"), 0o644)
	}
	return nil
}

// checkOrigin guards against a stale work dir: if the existing clone's origin
// doesn't match the configured repo, we'd silently push to the wrong place.
func (s *Store) checkOrigin() error {
	if s.repoURL == "" {
		return nil
	}
	origin, err := gitutil.Run(s.workDir, "remote", "get-url", "origin")
	if err != nil {
		return nil // no origin to compare against
	}
	if !sameRemote(origin, s.repoURL) {
		return fmt.Errorf(
			"work dir %s is a clone of %s, not the configured %s\nremove it to re-clone:  rm -rf %s",
			s.workDir, origin, s.repoURL, s.workDir)
	}
	return nil
}

// sameRemote compares two git URLs, ignoring transport/credentials/.git.
func sameRemote(a, b string) bool {
	ka, oka := domain.NormalizeRemote(a)
	kb, okb := domain.NormalizeRemote(b)
	if oka && okb {
		return ka == kb
	}
	return strings.TrimSuffix(a, ".git") == strings.TrimSuffix(b, ".git")
}

// RemoteHasContent reports whether the remote has any branches yet.
func (s *Store) RemoteHasContent() (bool, error) {
	return gitutil.RemoteHasBranches(s.workDir)
}

// Pull rebases the working copy onto the remote, auto-stashing local changes. If
// the rebase fails (e.g. a concurrent manifest edit on another device), it aborts
// the rebase so the working copy is left clean rather than mid-rebase.
func (s *Store) Pull() error {
	if err := gitutil.Stream(s.workDir, "pull", "--rebase", "--autostash"); err != nil {
		_ = gitutil.Stream(s.workDir, "rebase", "--abort") // best-effort cleanup
		return fmt.Errorf("pull failed and was rolled back (concurrent sync?): %w", err)
	}
	return nil
}

// Push stages everything, commits if there is anything to commit, and pushes,
// retrying once after a rebase if the remote moved underneath us.
func (s *Store) Push(message string) error {
	changed, err := gitutil.HasChanges(s.workDir)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	if _, err := gitutil.Run(s.workDir, "add", "-A"); err != nil {
		return err
	}
	if _, err := gitutil.Run(s.workDir, "commit", "-m", message); err != nil {
		return err
	}
	if err := s.push(); err != nil {
		if e := s.Pull(); e != nil {
			return fmt.Errorf("push rejected and rebase failed: %w", e)
		}
		if err := s.push(); err != nil {
			return fmt.Errorf("push failed after rebase: %w", err)
		}
	}
	return nil
}

// Delete removes a file from the working copy. The deletion is staged and
// published by the next Push (git add -A), which is how device removal reaches
// the remote on the git backend. It reports whether the file existed.
func (s *Store) Delete(rel string) (bool, error) {
	p := filepath.Join(s.workDir, filepath.FromSlash(rel))
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if err := os.Remove(p); err != nil {
		return false, err
	}
	return true, nil
}

// push sends the current branch, setting upstream on the first push.
func (s *Store) push() error {
	if gitutil.HasUpstream(s.workDir) {
		return gitutil.Stream(s.workDir, "push")
	}
	return gitutil.Stream(s.workDir, "push", "-u", "origin", "HEAD")
}
