// Package ghcli is a thin wrapper around the GitHub CLI (gh) for creating the
// private data repository during init.
package ghcli

import (
	"fmt"
	"os/exec"
	"strings"
)

// Available reports whether the gh CLI is installed.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// CreatePrivateRepo creates a private GitHub repo named name (e.g. "me/repo" or
// just "repo" for the current user) and returns its SSH clone URL.
func CreatePrivateRepo(name string) (string, error) {
	if !Available() {
		return "", fmt.Errorf("gh CLI not found; install it or pass --repo with an existing repo")
	}
	if out, err := run("repo", "create", name, "--private", "--clone=false"); err != nil {
		// Ignore "already exists" so init is re-runnable; surface anything else.
		if !strings.Contains(out, "already exists") {
			return "", fmt.Errorf("gh repo create: %s", strings.TrimSpace(out))
		}
	}
	url, err := run("repo", "view", name, "--json", "sshUrl", "--jq", ".sshUrl")
	if err != nil {
		return "", fmt.Errorf("gh repo view: %s", strings.TrimSpace(url))
	}
	return strings.TrimSpace(url), nil
}

func run(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
