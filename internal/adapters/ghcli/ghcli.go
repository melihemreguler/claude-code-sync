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

// IsPrivate reports whether a GitHub repo is private. known is false when it can't
// be determined (non-GitHub URL, gh missing, or the lookup failed).
func IsPrivate(gitURL string) (private bool, known bool) {
	slug := githubSlug(gitURL)
	if slug == "" || !Available() {
		return false, false
	}
	out, err := run("repo", "view", slug, "--json", "visibility", "--jq", ".visibility")
	if err != nil {
		return false, false
	}
	v := strings.TrimSpace(out)
	if v == "" {
		return false, false
	}
	return strings.EqualFold(v, "PRIVATE"), true
}

// githubSlug extracts "owner/repo" from a github.com git URL, or "" if the URL is
// not a recognizable github.com repo.
func githubSlug(gitURL string) string {
	u := strings.TrimSpace(gitURL)
	for _, p := range []string{"https://", "http://", "ssh://"} {
		u = strings.TrimPrefix(u, p)
	}
	u = strings.TrimPrefix(u, "git@")
	var rest string
	switch {
	case strings.HasPrefix(u, "github.com:"):
		rest = strings.TrimPrefix(u, "github.com:")
	case strings.HasPrefix(u, "github.com/"):
		rest = strings.TrimPrefix(u, "github.com/")
	default:
		return ""
	}
	rest = strings.Trim(strings.TrimSuffix(rest, ".git"), "/")
	if strings.Count(rest, "/") != 1 {
		return ""
	}
	return rest
}

func run(args ...string) (string, error) {
	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
