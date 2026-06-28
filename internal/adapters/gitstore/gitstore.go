// Package gitstore implements ports.Storage on top of a git repository, shelling
// out to the user's git so existing credentials and SSH config are reused.
package gitstore

import (
	"fmt"
	"os"
	"path/filepath"

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

// EnsureLocal clones the repo into workDir if it is not already a git work tree.
func (s *Store) EnsureLocal() error {
	if gitutil.IsRepo(s.workDir) {
		return nil
	}
	if s.repoURL == "" {
		return fmt.Errorf("no repository URL configured")
	}
	parent := filepath.Dir(s.workDir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	fmt.Printf("Cloning %s …\n", s.repoURL)
	return gitutil.Stream(parent, "clone", s.repoURL, s.workDir)
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

// push sends the current branch, setting upstream on the first push.
func (s *Store) push() error {
	if gitutil.HasUpstream(s.workDir) {
		return gitutil.Stream(s.workDir, "push")
	}
	return gitutil.Stream(s.workDir, "push", "-u", "origin", "HEAD")
}
