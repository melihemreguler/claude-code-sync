package cmd

import (
	"reflect"
	"testing"
)

func TestSplitComma(t *testing.T) {
	cases := map[string][]string{
		"~/dev/github, ~/work": {"~/dev/github", "~/work"},
		"  ~/a ,, ~/b ,  ":     {"~/a", "~/b"},
		"":                     nil,
		"   ":                  nil,
		"single":               {"single"},
	}
	for in, want := range cases {
		if got := splitComma(in); !reflect.DeepEqual(got, want) {
			t.Errorf("splitComma(%q) = %v, want %v", in, got, want)
		}
	}
}
