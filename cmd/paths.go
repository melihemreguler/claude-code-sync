package cmd

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveRoot expands a leading "~" and makes the path absolute, so include and
// exclude roots stored in config always compare correctly against absolute
// session working directories (a relative root would silently match nothing).
func resolveRoot(p string) string {
	home, _ := os.UserHomeDir()
	switch {
	case p == "~":
		p = home
	case strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, p[2:])
	}
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	return filepath.Clean(p)
}

func resolveRoots(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" {
			continue
		}
		out = append(out, resolveRoot(p))
	}
	return out
}
