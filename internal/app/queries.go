package app

import (
	"fmt"
	"os"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
)

// ProjectStatus describes a local project and whether it would sync.
type ProjectStatus struct {
	Folder   string
	Cwd      string
	Key      domain.CanonicalKey
	Included bool
}

// IncludeRoots returns the cleaned include roots in effect.
func (s *Syncer) IncludeRoots() []string { return s.include }

// ExcludeRoots returns the cleaned exclude roots in effect.
func (s *Syncer) ExcludeRoots() []string { return s.exclude }

// LocalProjects reports each local project's working directory, canonical key,
// and whether the current filter would sync it.
func (s *Syncer) LocalProjects() ([]ProjectStatus, error) {
	folders, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}
	out := make([]ProjectStatus, 0, len(folders))
	for _, f := range folders {
		cwd, err := s.store.ReadCwd(f)
		if err != nil {
			return nil, err
		}
		key, _ := s.ident.Key(cwd)
		out = append(out, ProjectStatus{
			Folder:   f,
			Cwd:      cwd,
			Key:      key,
			Included: cwd != "" && domain.IncludeCwd(cwd, s.include, s.exclude),
		})
	}
	return out, nil
}

// Manifest returns the current synced manifest after ensuring storage is local.
func (s *Syncer) Manifest() (*domain.Manifest, error) {
	if err := s.storage.EnsureLocal(); err != nil {
		return nil, err
	}
	return s.loadMerged()
}

// RemoveDevice drops a device from the chain by deleting its manifest shard, then
// publishes — under the sync lock.
//
// On the git backend the deletion is committed and pushed. On blob backends
// (S3/Drive) the Mirror is additive and does not delete remote objects yet, so the
// shard would reappear on the next sync — removal there needs a manual delete.
func (s *Syncer) RemoveDevice(name string) (bool, error) {
	removed := false
	err := s.withLock(func() error {
		if err := s.EnsureReady(); err != nil {
			return err
		}
		if err := s.refresh(); err != nil {
			return err
		}
		path := s.shardPath(name)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil // no shard → nothing to remove
		} else if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		if err := s.storage.Push(fmt.Sprintf("device: remove %s", name)); err != nil {
			return err
		}
		removed = true
		return nil
	})
	return removed, err
}
