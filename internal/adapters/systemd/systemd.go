// Package systemd installs ccsync auto-sync agents as systemd **user** units on
// Linux — a periodic sync (a .service driven by a .timer) and/or a keep-alive
// `ccsync watch` (.service with Restart=always). It is the Linux counterpart to
// the launchd adapter; cmd selects between them by GOOS.
package systemd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Spec describes a user unit to install. It mirrors launchd.Spec so the two
// adapters are interchangeable from the caller's point of view.
type Spec struct {
	Label       string   // unit base name (e.g. "com.ccsync.sync")
	Args        []string // arguments passed to the ccsync binary
	IntervalSec int      // periodic interval; >0 installs a .timer
	KeepAlive   bool     // restart if it exits (for `watch`)
	LogPath     string   // stdout/stderr destination
}

// unitDir returns the per-user systemd unit directory.
func unitDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

// servicePath / timerPath return a label's unit file locations.
func servicePath(label string) (string, error) {
	dir, err := unitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, label+".service"), nil
}

func timerPath(label string) (string, error) {
	dir, err := unitDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, label+".timer"), nil
}

// Installed reports whether a label's service or timer unit exists.
func Installed(label string) bool {
	for _, fn := range []func(string) (string, error){servicePath, timerPath} {
		if p, err := fn(label); err == nil {
			if _, err := os.Stat(p); err == nil {
				return true
			}
		}
	}
	return false
}

// Install writes spec's unit file(s) and enables them via systemctl --user.
func Install(exe string, spec Spec) error {
	dir, err := unitDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	svcPath, _ := servicePath(spec.Label)
	if err := os.WriteFile(svcPath, []byte(ServiceContent(exe, spec)), 0o644); err != nil {
		return err
	}

	unit := spec.Label + ".service"
	if spec.IntervalSec > 0 {
		tPath, _ := timerPath(spec.Label)
		if err := os.WriteFile(tPath, []byte(TimerContent(spec)), 0o644); err != nil {
			return err
		}
		unit = spec.Label + ".timer"
	}

	_ = systemctl("daemon-reload")
	return systemctl("enable", "--now", unit)
}

// Remove disables and deletes a label's units.
func Remove(label string) error {
	_ = systemctl("disable", "--now", label+".timer")
	_ = systemctl("disable", "--now", label+".service")
	for _, fn := range []func(string) (string, error){servicePath, timerPath} {
		p, err := fn(label)
		if err != nil {
			return err
		}
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	_ = systemctl("daemon-reload")
	return nil
}

// ServiceContent renders the .service unit (pure, for testing). A periodic spec
// is a oneshot run; a keep-alive spec restarts on exit.
func ServiceContent(exe string, spec Spec) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=ccsync %s\n\n", strings.Join(spec.Args, " "))
	b.WriteString("[Service]\n")
	if spec.KeepAlive {
		b.WriteString("Restart=always\nRestartSec=5\n")
	} else {
		b.WriteString("Type=oneshot\n")
	}
	fmt.Fprintf(&b, "ExecStart=%s\n", execStart(exe, spec.Args))
	if spec.LogPath != "" {
		fmt.Fprintf(&b, "StandardOutput=append:%s\n", spec.LogPath)
		fmt.Fprintf(&b, "StandardError=append:%s\n", spec.LogPath)
	}
	// A oneshot service driven by a timer needs no [Install]; a keep-alive
	// service is enabled directly, so it must declare one.
	if spec.KeepAlive {
		b.WriteString("\n[Install]\nWantedBy=default.target\n")
	}
	return b.String()
}

// TimerContent renders the .timer unit that drives a periodic service (pure).
func TimerContent(spec Spec) string {
	var b strings.Builder
	b.WriteString("[Unit]\n")
	fmt.Fprintf(&b, "Description=ccsync %s timer\n\n", strings.Join(spec.Args, " "))
	b.WriteString("[Timer]\n")
	fmt.Fprintf(&b, "OnBootSec=%ds\n", spec.IntervalSec)
	fmt.Fprintf(&b, "OnUnitActiveSec=%ds\n", spec.IntervalSec)
	b.WriteString("Persistent=true\n\n")
	b.WriteString("[Install]\nWantedBy=timers.target\n")
	return b.String()
}

// execStart joins the binary and its arguments into an ExecStart line.
func execStart(exe string, args []string) string {
	if len(args) == 0 {
		return exe
	}
	return exe + " " + strings.Join(args, " ")
}

func systemctl(args ...string) error {
	return exec.Command("systemctl", append([]string{"--user"}, args...)...).Run()
}
