// Package launchd installs ccsync auto-sync agents as macOS LaunchAgents — a
// periodic `ccsync sync` and/or a keep-alive `ccsync watch`.
package launchd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Spec describes a LaunchAgent to install.
type Spec struct {
	Label       string   // reverse-DNS identifier
	Args        []string // arguments passed to the ccsync binary
	IntervalSec int      // StartInterval; 0 to omit
	KeepAlive   bool     // restart if it exits (for `watch`)
	LogPath     string   // stdout/stderr destination
}

// PlistPath returns a label's LaunchAgent plist location for the current user.
func PlistPath(label string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist"), nil
}

// Installed reports whether a label's plist exists.
func Installed(label string) bool {
	p, err := PlistPath(label)
	if err != nil {
		return false
	}
	_, err = os.Stat(p)
	return err == nil
}

// Install writes the plist for spec and loads the agent.
func Install(exe string, spec Spec) error {
	p, err := PlistPath(spec.Label)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(p, []byte(PlistContent(exe, spec)), 0o644); err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", p).Run()
	return exec.Command("launchctl", "load", "-w", p).Run()
}

// Remove unloads and deletes a label's LaunchAgent.
func Remove(label string) error {
	p, err := PlistPath(label)
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "unload", p).Run()
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// PlistContent renders a LaunchAgent plist (pure, for testing).
func PlistContent(exe string, spec Spec) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", spec.Label)
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	fmt.Fprintf(&b, "\t\t<string>%s</string>\n", exe)
	for _, a := range spec.Args {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", a)
	}
	b.WriteString("\t</array>\n")
	if spec.IntervalSec > 0 {
		fmt.Fprintf(&b, "\t<key>StartInterval</key>\n\t<integer>%d</integer>\n", spec.IntervalSec)
	}
	if spec.KeepAlive {
		b.WriteString("\t<key>KeepAlive</key>\n\t<true/>\n")
	}
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n")
	fmt.Fprintf(&b, "\t<key>StandardOutPath</key>\n\t<string>%s</string>\n", spec.LogPath)
	fmt.Fprintf(&b, "\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", spec.LogPath)
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}
