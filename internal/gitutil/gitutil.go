// Package gitutil is a thin wrapper around the git CLI. ccsync shells out to git
// rather than embedding a git library to keep the binary dependency-light and to
// reuse the user's existing credentials and SSH configuration.
package gitutil

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// Run executes a git command in dir and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errBuf.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(out.String()), nil
}

// Stream executes a git command with stdout/stderr attached to the terminal,
// for commands whose progress the user should see (clone, push, pull).
func Stream(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// IsRepo reports whether dir is inside a git working tree.
func IsRepo(dir string) bool {
	_, err := Run(dir, "rev-parse", "--is-inside-work-tree")
	return err == nil
}

// HasChanges reports whether the working tree has staged or unstaged changes.
func HasChanges(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out != "", nil
}

// HasUpstream reports whether the current branch has a configured upstream.
func HasUpstream(dir string) bool {
	_, err := Run(dir, "rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

// RemoteHasBranches reports whether the remote already has any branches. A
// brand-new (empty) data repo has none, in which case there is nothing to pull.
func RemoteHasBranches(dir string) (bool, error) {
	out, err := Run(dir, "ls-remote", "--heads", "origin")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
