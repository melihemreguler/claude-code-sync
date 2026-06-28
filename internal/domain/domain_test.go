package domain

import "testing"

func TestEncodeCwd(t *testing.T) {
	cases := map[string]string{
		"/Users/me/dev/github/foo":       "-Users-me-dev-github-foo",
		"/Users/me/.claude-mem/observer": "-Users-me--claude-mem-observer",
		"/Users/me/dev/github/acme/cli":  "-Users-me-dev-github-acme-cli",
	}
	for in, want := range cases {
		if got := EncodeCwd(in); got != want {
			t.Errorf("EncodeCwd(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeRemote_UnifiesForms(t *testing.T) {
	want := CanonicalKey("github.com/acme/widgets")
	forms := []string{
		"git@github.com:acme/widgets.git",
		"https://github.com/acme/widgets.git",
		"https://github.com/acme/widgets",
		"ssh://git@github.com/acme/widgets.git",
		"git@GitHub.com:acme/widgets.git", // host case-insensitive
	}
	for _, f := range forms {
		got, ok := NormalizeRemote(f)
		if !ok || got != want {
			t.Errorf("NormalizeRemote(%q) = %q,%v; want %q", f, got, ok, want)
		}
	}
	if _, ok := NormalizeRemote("  "); ok {
		t.Error("empty remote should not be ok")
	}
}

// The crux of P1: the same repo at different absolute paths on different machines
// must resolve to the same canonical key.
func TestCanonicalKey_CrossDevice(t *testing.T) {
	deviceA := "git@github.com:acme/widgets.git"     // ~/dev/github/widgets, user melihguler
	deviceB := "https://github.com/acme/widgets.git" // ~/github/widgets, user other
	ka, _ := NormalizeRemote(deviceA)
	kb, _ := NormalizeRemote(deviceB)
	if ka != kb {
		t.Fatalf("same repo gave different keys: %q vs %q", ka, kb)
	}
}

func TestFallbackKey_HomeIndependent(t *testing.T) {
	a := FallbackKey("/Users/melihguler/notes/journal", "/Users/melihguler")
	b := FallbackKey("/Users/other/notes/journal", "/Users/other")
	if a != b {
		t.Errorf("fallback key should ignore home/username: %q vs %q", a, b)
	}
	if a != "path:~/notes/journal" {
		t.Errorf("unexpected fallback key %q", a)
	}
}

func TestIncludeCwd(t *testing.T) {
	inc := []string{"/Users/me/dev/github"}
	exc := []string{"/Users/me/dev/github/secret"}
	tests := []struct {
		cwd  string
		want bool
	}{
		{"/Users/me/dev/github/foo", true},
		{"/Users/me/dev/github", true},         // root itself
		{"/Users/me/dev/github/secret", false}, // excluded root
		{"/Users/me/dev/github/secret/x", false},
		{"/Users/me/dev/gitlab/foo", false},       // outside include
		{"/Users/me/dev/github-extra/foo", false}, // prefix must be a path boundary
	}
	for _, tt := range tests {
		if got := IncludeCwd(tt.cwd, inc, exc); got != tt.want {
			t.Errorf("IncludeCwd(%q) = %v, want %v", tt.cwd, got, tt.want)
		}
	}
	if IncludeCwd("/anything", nil, nil) {
		t.Error("empty include must sync nothing")
	}
}

func TestCleanRoot(t *testing.T) {
	home := "/Users/me"
	cases := map[string]string{
		"~":             "/Users/me",
		"~/dev/github/": "/Users/me/dev/github",
		"/abs/path/":    "/abs/path",
	}
	for in, want := range cases {
		if got := CleanRoot(in, home); got != want {
			t.Errorf("CleanRoot(%q) = %q, want %q", in, got, want)
		}
	}
}
