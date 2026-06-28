package gitstore

import "testing"

func TestSameRemote(t *testing.T) {
	same := [][2]string{
		{"git@github.com:me/data.git", "https://github.com/me/data.git"},
		{"git@github.com:me/data.git", "git@github.com:me/data"},
		{"ssh://git@github.com/me/data", "https://github.com/me/data.git"},
	}
	for _, p := range same {
		if !sameRemote(p[0], p[1]) {
			t.Errorf("expected same remote: %q vs %q", p[0], p[1])
		}
	}
	if sameRemote("git@github.com:me/data.git", "git@github.com:me/other.git") {
		t.Error("different repos must not be treated as the same remote")
	}
}
