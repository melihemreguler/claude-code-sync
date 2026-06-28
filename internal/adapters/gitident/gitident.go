// Package gitident implements ports.Identifier by deriving a project's canonical
// key from its git remote, falling back to a home-relative path key.
package gitident

import (
	"os"
	"path/filepath"

	"github.com/melihemreguler/claude-code-sync/internal/domain"
	"github.com/melihemreguler/claude-code-sync/internal/gitutil"
)

// Identifier resolves canonical keys for projects on this machine.
type Identifier struct {
	home string
}

// New returns an Identifier that uses home to make fallback keys portable.
func New(home string) *Identifier {
	return &Identifier{home: home}
}

// Key returns the canonical key and display name for the project at cwd. It uses
// the git remote when available (path-independent across machines), otherwise a
// home-relative path key.
func (i *Identifier) Key(cwd string) (domain.CanonicalKey, string) {
	if cwd == "" {
		return "", ""
	}
	if remote, ok := gitRemote(cwd); ok {
		if key, ok := domain.NormalizeRemote(remote); ok {
			return key, filepath.Base(string(key))
		}
	}
	return domain.FallbackKey(cwd, i.home), filepath.Base(cwd)
}

// gitRemote returns origin's URL for the repo at dir. It returns false if dir
// does not exist or is not a git work tree with an origin remote.
func gitRemote(dir string) (string, bool) {
	if _, err := os.Stat(dir); err != nil {
		return "", false
	}
	out, err := gitutil.Run(dir, "remote", "get-url", "origin")
	if err != nil || out == "" {
		return "", false
	}
	return out, true
}
