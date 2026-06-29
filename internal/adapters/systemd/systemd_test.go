package systemd

import (
	"strings"
	"testing"
)

func TestServiceContentPeriodic(t *testing.T) {
	got := ServiceContent("/usr/bin/ccsync", Spec{
		Label:       "com.ccsync.sync",
		Args:        []string{"sync"},
		IntervalSec: 900,
		LogPath:     "/tmp/ccsync.log",
	})
	for _, want := range []string{
		"Type=oneshot",
		"ExecStart=/usr/bin/ccsync sync",
		"StandardOutput=append:/tmp/ccsync.log",
		"StandardError=append:/tmp/ccsync.log",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("service unit missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Restart=") {
		t.Error("periodic (oneshot) service must not set Restart")
	}
	if strings.Contains(got, "[Install]") {
		t.Error("timer-driven oneshot service must not declare [Install]")
	}
}

func TestServiceContentWatchKeepAlive(t *testing.T) {
	got := ServiceContent("/usr/bin/ccsync", Spec{
		Label:     "com.ccsync.watch",
		Args:      []string{"watch"},
		KeepAlive: true,
		LogPath:   "/tmp/ccsync.log",
	})
	for _, want := range []string{
		"Restart=always",
		"ExecStart=/usr/bin/ccsync watch",
		"[Install]",
		"WantedBy=default.target",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("watch service missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Type=oneshot") {
		t.Error("keep-alive service must not be oneshot")
	}
}

func TestTimerContent(t *testing.T) {
	got := TimerContent(Spec{Label: "com.ccsync.sync", Args: []string{"sync"}, IntervalSec: 900})
	for _, want := range []string{
		"OnBootSec=900s",
		"OnUnitActiveSec=900s",
		"Persistent=true",
		"WantedBy=timers.target",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("timer unit missing %q:\n%s", want, got)
		}
	}
}

func TestExecStartNoArgs(t *testing.T) {
	if got := execStart("/usr/bin/ccsync", nil); got != "/usr/bin/ccsync" {
		t.Errorf("execStart with no args = %q", got)
	}
}
