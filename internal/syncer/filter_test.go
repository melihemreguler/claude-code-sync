package syncer

import "testing"

func TestMatchFilter(t *testing.T) {
	tests := []struct {
		name    string
		folder  string
		include []string
		exclude []string
		want    bool
	}{
		{"github included by default-style glob", "-Users-me-dev-github-foo", []string{"*github*"}, nil, true},
		{"work repo excluded by not matching include", "-Users-me-turknet-secret", []string{"*github*"}, nil, false},
		{"exclude beats include", "-Users-me-dev-github-work", []string{"*github*"}, []string{"*work*"}, false},
		{"empty include means all", "-Users-me-anything", nil, nil, true},
		{"empty include but excluded", "-Users-me-turknet-x", nil, []string{"*turknet*"}, false},
		{"multiple includes, second matches", "-Users-me-personal-blog", []string{"*github*", "*personal*"}, nil, true},
		{"no include match", "-Users-me-random", []string{"*github*"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchFilter(tt.folder, tt.include, tt.exclude); got != tt.want {
				t.Errorf("MatchFilter(%q, %v, %v) = %v, want %v",
					tt.folder, tt.include, tt.exclude, got, tt.want)
			}
		})
	}
}
