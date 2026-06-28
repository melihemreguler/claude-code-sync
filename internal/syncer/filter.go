package syncer

import "path/filepath"

// MatchFilter reports whether a project folder name should be synced, given the
// include/exclude glob patterns. Exclude always wins.
//
// An empty include list means "sync nothing" — this is a deliberate safety
// choice so that removing your last include pattern never silently starts
// syncing every project (including work repos). To include everything, use an
// explicit "*" pattern.
func MatchFilter(name string, include, exclude []string) bool {
	for _, p := range exclude {
		if ok, _ := filepath.Match(p, name); ok {
			return false
		}
	}
	for _, p := range include {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}
