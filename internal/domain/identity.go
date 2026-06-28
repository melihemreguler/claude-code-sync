package domain

import "strings"

// CanonicalKey is a stable, path-independent identity for a logical project,
// shared by every device that has it. Two checkouts of the same git repository
// resolve to the same CanonicalKey even when they live at different absolute
// paths or under different usernames — this is what makes cross-device path
// differences transparent.
type CanonicalKey string

// NormalizeRemote turns a git remote URL into a canonical key by stripping the
// transport, credentials and ".git" suffix and lowercasing the host. All of
// these map to "github.com/acme/widgets":
//
//	git@github.com:acme/widgets.git
//	https://github.com/acme/widgets.git
//	ssh://git@github.com/acme/widgets
//
// It returns ("", false) if the input is empty.
func NormalizeRemote(remote string) (CanonicalKey, bool) {
	s := strings.TrimSpace(remote)
	if s == "" {
		return "", false
	}
	// Strip known scheme prefixes.
	for _, p := range []string{"ssh://", "https://", "http://", "git://"} {
		s = strings.TrimPrefix(s, p)
	}
	// Strip user@ credentials.
	if at := strings.LastIndex(s, "@"); at != -1 {
		s = s[at+1:]
	}
	// scp-style "host:path" → "host/path".
	s = strings.Replace(s, ":", "/", 1)
	s = strings.TrimSuffix(s, ".git")
	s = strings.Trim(s, "/")

	// Lowercase only the host component; repo paths can be case-sensitive.
	if slash := strings.Index(s, "/"); slash != -1 {
		s = strings.ToLower(s[:slash]) + s[slash:]
	} else {
		s = strings.ToLower(s)
	}
	if s == "" {
		return "", false
	}
	return CanonicalKey(s), true
}

// FallbackKey builds a key for projects without a usable git remote. It is based
// on the home-relative working directory, so it survives a differing home dir or
// username but NOT a different directory layout (e.g. ~/dev/github vs ~/github).
// Such projects simply do not auto-translate across structurally different
// machines; git-backed projects do.
func FallbackKey(cwd, home string) CanonicalKey {
	rel := HomeRelative(cwd, home)
	return CanonicalKey("path:" + rel)
}

// HomeRelative rewrites a leading home directory to "~" so paths compare equally
// across machines with different usernames.
func HomeRelative(path, home string) string {
	if home != "" && (path == home || strings.HasPrefix(path, home+"/")) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}
