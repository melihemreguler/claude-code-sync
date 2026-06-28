// Package domain holds ccsync's pure business rules: project identity, path
// encoding, and filtering. It performs no I/O and imports nothing internal, so
// it can be reasoned about and tested in isolation.
package domain

import "strings"

// EncodeCwd reproduces how Claude Code names the per-project directory under
// ~/.claude/projects: every "/" and "." in the absolute working directory is
// replaced with "-".
//
// The transform is lossy (both "/" and "." collapse to "-"), so it cannot be
// reversed. That is precisely why ccsync reads the true working directory from
// the session file's "cwd" field rather than trying to decode the folder name.
func EncodeCwd(cwd string) string {
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(cwd)
}
