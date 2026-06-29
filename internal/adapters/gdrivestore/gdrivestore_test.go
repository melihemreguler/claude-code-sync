package gdrivestore

import "testing"

func TestTrimNewline(t *testing.T) {
	cases := map[string]string{
		"abc\n":   "abc",
		"abc\r\n": "abc",
		"abc":     "abc",
		"":        "",
		"\n":      "",
	}
	for in, want := range cases {
		if got := trimNewline(in); got != want {
			t.Errorf("trimNewline(%q) = %q, want %q", in, got, want)
		}
	}
}
