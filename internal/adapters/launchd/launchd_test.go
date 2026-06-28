package launchd

import (
	"strings"
	"testing"
)

func TestPlistContentPeriodic(t *testing.T) {
	got := PlistContent("/usr/local/bin/ccsync", Spec{
		Label:       "com.ccsync.sync",
		Args:        []string{"sync"},
		IntervalSec: 900,
		LogPath:     "/tmp/ccsync.log",
	})
	for _, want := range []string{
		"<string>com.ccsync.sync</string>",
		"<string>/usr/local/bin/ccsync</string>",
		"<string>sync</string>",
		"<integer>900</integer>",
		"/tmp/ccsync.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("plist missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "KeepAlive") {
		t.Error("periodic agent should not set KeepAlive")
	}
}

func TestPlistContentWatchKeepAlive(t *testing.T) {
	got := PlistContent("/usr/local/bin/ccsync", Spec{
		Label:     "com.ccsync.watch",
		Args:      []string{"watch"},
		KeepAlive: true,
		LogPath:   "/tmp/ccsync.log",
	})
	if !strings.Contains(got, "<key>KeepAlive</key>") {
		t.Error("watch agent should set KeepAlive")
	}
	if strings.Contains(got, "StartInterval") {
		t.Error("watch agent should not set StartInterval")
	}
}
