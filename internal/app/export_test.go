package app

// SetLockDir overrides the sync lock directory. It exists so tests can point the
// flock at an isolated temp dir, keeping them hermetic from a real background
// ccsync auto-sync that may hold the lock in the per-user config dir.
func (s *Syncer) SetLockDir(dir string) { s.lockDir = dir }
