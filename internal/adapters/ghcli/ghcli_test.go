package ghcli

import "testing"

func TestGithubSlug(t *testing.T) {
	cases := map[string]string{
		"git@github.com:me/data.git":       "me/data",
		"https://github.com/me/data.git":   "me/data",
		"https://github.com/me/data":       "me/data",
		"ssh://git@github.com/me/data.git": "me/data",
		"git@gitlab.com:me/data.git":       "", // not github
		"git@github.com:me/data/extra.git": "", // not owner/repo
		"":                                 "",
	}
	for in, want := range cases {
		if got := githubSlug(in); got != want {
			t.Errorf("githubSlug(%q) = %q, want %q", in, got, want)
		}
	}
}
