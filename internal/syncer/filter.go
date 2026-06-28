package syncer

import "path/filepath"

// MatchFilter reports whether a project folder name should be synced, given the
// include/exclude glob patterns. Exclude always wins. An empty include list
// means "everything not excluded".
func MatchFilter(name string, include, exclude []string) bool {
	for _, p := range exclude {
		if ok, _ := filepath.Match(p, name); ok {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, p := range include {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}
