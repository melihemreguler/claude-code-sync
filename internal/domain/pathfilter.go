package domain

import "path/filepath"

// IncludeCwd reports whether a project's working directory should be synced,
// given include/exclude directory roots. A project is synced when its cwd lies
// within an include root and within no exclude root. Exclude always wins.
//
// An empty include list means "sync nothing" — the same deliberate safety choice
// as the rest of ccsync: nothing is ever synced unless explicitly opted in.
//
// Callers pass already-cleaned absolute paths (see CleanRoot).
func IncludeCwd(cwd string, include, exclude []string) bool {
	for _, root := range exclude {
		if within(cwd, root) {
			return false
		}
	}
	for _, root := range include {
		if within(cwd, root) {
			return true
		}
	}
	return false
}

// within reports whether path is root itself or nested under it.
func within(path, root string) bool {
	if path == root {
		return true
	}
	return len(path) > len(root) && path[:len(root)] == root && path[len(root)] == filepath.Separator
}

// CleanRoot normalizes a user-supplied directory: it expands a leading "~" using
// home and returns a cleaned absolute-style path with no trailing separator.
func CleanRoot(root, home string) string {
	if root == "~" {
		root = home
	} else if len(root) >= 2 && root[:2] == "~/" {
		root = filepath.Join(home, root[2:])
	}
	return filepath.Clean(root)
}
