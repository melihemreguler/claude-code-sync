package app

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
	"github.com/melihemreguler/claude-code-sync/internal/fileutil"
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

// GCResult tallies a garbage-collection pass.
type GCResult struct {
	Orphans int
	Freed   int64 // bytes reclaimed
}

// GC deletes encrypted object blobs in storage that no manifest shard references
// — for example, the objects left behind after `device remove` drops the only
// device that had a project. It never prunes the manifest itself, so any session
// still listed by some device is kept. With dryRun it reports what it would
// delete without changing anything.
//
// Safety: the live set is built from the fully merged manifest, and a shard that
// fails to decrypt aborts the whole pass (loadMerged errors) rather than letting
// its objects look unreferenced.
func (s *Syncer) GC(dryRun bool) (GCResult, error) {
	var res GCResult
	err := s.withLock(func() error {
		if err := s.EnsureReady(); err != nil {
			return err
		}
		if err := s.refresh(); err != nil {
			return err
		}
		merged, err := s.loadMerged()
		if err != nil {
			return err
		}

		live := map[string]bool{}
		for keyStr, entry := range merged.Projects {
			for rel := range entry.Objects {
				live[s.objectRel(domain.CanonicalKey(keyStr), rel)] = true
			}
		}

		objectsRoot := filepath.Join(s.storage.RootDir(), objectsDir)
		var orphans []string
		walkErr := filepath.WalkDir(objectsRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if d.IsDir() || strings.HasSuffix(d.Name(), fileutil.TmpSuffix) {
				return nil
			}
			relOS, err := filepath.Rel(s.storage.RootDir(), path)
			if err != nil {
				return err
			}
			rel := filepath.ToSlash(relOS)
			if live[rel] {
				return nil
			}
			if info, err := d.Info(); err == nil {
				res.Freed += info.Size()
			}
			orphans = append(orphans, rel)
			return nil
		})
		if walkErr != nil {
			return walkErr
		}

		res.Orphans = len(orphans)
		if dryRun || len(orphans) == 0 {
			return nil
		}
		for _, rel := range orphans {
			if _, err := s.storage.Delete(rel); err != nil {
				return err
			}
		}
		return s.storage.Push(fmt.Sprintf("gc: remove %d orphaned object(s)", len(orphans)))
	})
	return res, err
}

// RemoveDevice drops a device from the chain by deleting its manifest shard, then
// publishes — under the sync lock.
//
// Deletion goes through the Storage port, so it works on every backend: the git
// backend stages and pushes the removal, while blob backends (S3/Drive) delete
// the remote shard directly. It returns false if the device had no shard.
func (s *Syncer) RemoveDevice(name string) (bool, error) {
	removed := false
	err := s.withLock(func() error {
		if err := s.EnsureReady(); err != nil {
			return err
		}
		if err := s.refresh(); err != nil {
			return err
		}
		existed, err := s.storage.Delete(shardRel(name))
		if err != nil {
			return err
		}
		if !existed {
			return nil // no shard → nothing to remove
		}
		if err := s.storage.Push(fmt.Sprintf("device: remove %s", name)); err != nil {
			return err
		}
		removed = true
		return nil
	})
	return removed, err
}
