package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"
)

// isInteractive reports whether stdin is a real terminal (so a tour makes sense).
// This is false for pipes and for /dev/null, which a char-device check would
// wrongly accept.
func isInteractive() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// runTour collects init choices interactively and populates the init* flags, so
// the rest of runInit proceeds exactly as it would from flags. Every choice has a
// flag equivalent, keeping non-interactive/CI use (--no-input) fully supported.
func runTour() error {
	fmt.Println("Welcome to ccsync — let's set up syncing for this device.")

	if initDevice == "" {
		initDevice, _ = os.Hostname()
	}
	chainMode := "new"
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Device name").Value(&initDevice),
		huh.NewSelect[string]().Title("Storage backend").Value(&initBackend).Options(
			huh.NewOption("Git repository", "git"),
			huh.NewOption("Amazon S3", "s3"),
			huh.NewOption("Google Drive", "gdrive"),
		),
		huh.NewSelect[string]().Title("Encrypted chain").Value(&chainMode).Options(
			huh.NewOption("Start a new chain", "new"),
			huh.NewOption("Join an existing chain", "join"),
		),
	)).Run(); err != nil {
		return err
	}
	initNewChain = chainMode == "new"
	initJoin = chainMode == "join"

	if err := tourBackend(); err != nil {
		return err
	}
	if initJoin {
		if err := tourJoin(); err != nil {
			return err
		}
	}
	return tourSelection()
}

func tourBackend() error {
	switch initBackend {
	case "git":
		// Joining a chain means its data repo already exists — only ask for its
		// URL. Creating a new repo only makes sense when starting a new chain.
		if initJoin {
			return huh.NewForm(huh.NewGroup(
				huh.NewInput().Title("Existing chain's git URL (git@github.com:you/claude-sessions.git)").
					Validate(notEmpty("a git URL")).Value(&initRepo),
			)).Run()
		}
		repoMode := "existing"
		if err := huh.NewForm(huh.NewGroup(
			huh.NewSelect[string]().Title("Repository").Value(&repoMode).Options(
				huh.NewOption("Use an existing private repo", "existing"),
				huh.NewOption("Create a new private GitHub repo (via gh)", "create"),
			),
		)).Run(); err != nil {
			return err
		}
		field := huh.NewInput().Title("Git URL (git@github.com:you/claude-sessions.git)").
			Validate(notEmpty("a git URL")).Value(&initRepo)
		if repoMode == "create" {
			field = huh.NewInput().Title("New repo name (e.g. claude-sessions)").
				Validate(notEmpty("a repo name")).Value(&initCreateRepo)
		}
		return huh.NewForm(huh.NewGroup(field)).Run()
	case "s3":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("S3 bucket").Validate(notEmpty("a bucket")).Value(&initS3Bucket),
			huh.NewInput().Title("Key prefix").Value(&initS3Prefix),
			huh.NewInput().Title("Region (blank = AWS default)").Value(&initS3Region),
		)).Run()
	case "gdrive":
		return huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Drive folder ID").Validate(notEmpty("a folder ID")).Value(&initGDriveFolder),
			huh.NewInput().Title("OAuth client secret JSON path").Validate(notEmpty("the credentials path")).Value(&initGDriveCreds),
		)).Run()
	}
	return nil
}

// notEmpty returns a huh validator that rejects blank input.
func notEmpty(what string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("please enter %s", what)
		}
		return nil
	}
}

func tourJoin() error {
	mode := "merge"
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title("Chain identity — from `ccsync key show` on another device").
			EchoMode(huh.EchoModePassword).
			Validate(notEmpty("a chain identity")).
			Value(&initKey),
		huh.NewSelect[string]().Title("First sync").Value(&mode).Options(
			huh.NewOption("Merge: combine this machine's history with the chain", "merge"),
			huh.NewOption("Claude-base: publish this machine's history, don't import the chain's yet", "claude-base"),
		),
	)).Run(); err != nil {
		return err
	}
	initClaudeBase = mode == "claude-base"
	return nil
}

func tourSelection() error {
	includeStr := strings.Join(initInclude, ", ")
	var triggers []string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Directories to sync (comma-separated, e.g. ~/dev/github)").Value(&includeStr),
		huh.NewMultiSelect[string]().Title("Auto-sync triggers (optional)").Value(&triggers).Options(
			huh.NewOption("On session start/end (hooks)", "hooks"),
			huh.NewOption("Periodically (launchd)", "launchd"),
			huh.NewOption("Real-time (watcher)", "watch"),
		),
	)).Run(); err != nil {
		return err
	}
	initInclude = splitComma(includeStr)
	autoHooks = contains(triggers, "hooks")
	autoLaunchd = contains(triggers, "launchd")
	autoWatch = contains(triggers, "watch")
	return nil
}

func splitComma(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
