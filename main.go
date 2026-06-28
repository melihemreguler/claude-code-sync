package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "sync":
		err = cmdSync(args)
	case "push":
		err = cmdPush(args)
	case "pull":
		err = cmdPull(args)
	case "status":
		err = cmdStatus(args)
	case "device":
		err = cmdDevice(args)
	case "filter":
		err = cmdFilter(args)
	case "version", "--version", "-v":
		fmt.Printf("ccsync %s\n", version)
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`ccsync — selective, multi-device sync for Claude Code sessions

Usage:
  ccsync init --repo <git-url> [--device <name>] [--include <globs>] [--exclude <globs>]
  ccsync sync                 Pull remote changes, then push local ones (default workflow)
  ccsync pull                 Apply remote sessions into ~/.claude
  ccsync push                 Send local sessions to the remote
  ccsync status               Show config, device chain, and what matches the filter
  ccsync device list          Show devices in the sync chain (the control panel)
  ccsync device remove <name> Drop a device from the chain
  ccsync filter list          Show include/exclude patterns
  ccsync filter add    --include <glob> | --exclude <glob>
  ccsync filter remove --include <glob> | --exclude <glob>
  ccsync version

Filters match Claude project folder names, which embed the full path
(e.g. "-Users-me-dev-github-foo"). Default include is "*github*".
`)
}

// splitGlobs turns "a,b , c" into ["a","b","c"].
func splitGlobs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	repo := fs.String("repo", "", "git URL of the PRIVATE data repo (required)")
	device := fs.String("device", "", "device name (default: hostname)")
	include := fs.String("include", "*github*", "comma-separated include globs")
	exclude := fs.String("exclude", "", "comma-separated exclude globs")
	claudeDir := fs.String("claude-dir", defaultClaudeDir(), "Claude Code home directory")
	workDir := fs.String("work-dir", defaultWorkDir(), "local clone location for the data repo")
	_ = fs.Parse(args)

	if *repo == "" {
		return fmt.Errorf("--repo is required (create a PRIVATE github repo and pass its git URL)")
	}
	name := *device
	if name == "" {
		host, _ := os.Hostname()
		name = host
	}
	if name == "" {
		return fmt.Errorf("could not determine device name; pass --device")
	}

	cfg := &Config{
		Device:    name,
		RepoURL:   *repo,
		ClaudeDir: *claudeDir,
		WorkDir:   *workDir,
		Include:   splitGlobs(*include),
		Exclude:   splitGlobs(*exclude),
	}
	if err := cfg.save(); err != nil {
		return err
	}
	if err := ensureRepo(cfg); err != nil {
		return err
	}
	if err := seedRepo(cfg); err != nil {
		return err
	}

	// Register this device and do a first sync so the chain is established.
	reg, err := loadRegistry(cfg.WorkDir)
	if err != nil {
		return err
	}
	reg.upsert(cfg.Device, platform())
	if err := reg.save(cfg.WorkDir); err != nil {
		return err
	}

	fmt.Printf("Initialized device %q.\n", cfg.Device)
	fmt.Printf("  data repo:  %s\n", cfg.RepoURL)
	fmt.Printf("  include:    %v\n", cfg.Include)
	fmt.Printf("  exclude:    %v\n", cfg.Exclude)
	fmt.Println("Running first sync …")
	return runSync(cfg)
}

// seedRepo makes sure the data repo has the structure ccsync expects:
// a projects/ dir and a .gitattributes that union-merges JSONL session logs so
// concurrent appends from two devices auto-merge instead of conflicting.
func seedRepo(cfg *Config) error {
	if err := os.MkdirAll(filepath.Join(cfg.WorkDir, projectsSubdir), 0o755); err != nil {
		return err
	}
	keep := filepath.Join(cfg.WorkDir, projectsSubdir, ".gitkeep")
	if _, err := os.Stat(keep); os.IsNotExist(err) {
		if err := os.WriteFile(keep, []byte{}, 0o644); err != nil {
			return err
		}
	}
	attr := filepath.Join(cfg.WorkDir, ".gitattributes")
	if _, err := os.Stat(attr); os.IsNotExist(err) {
		content := "# Session logs are append-only — union-merge concurrent edits.\n*.jsonl merge=union\n"
		if err := os.WriteFile(attr, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func cmdSync(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return runSync(cfg)
}

func runSync(cfg *Config) error {
	in, err := pull(cfg)
	if err != nil {
		return err
	}
	out, err := push(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.files, out.files)
	return nil
}

func cmdPull(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	res, err := pull(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Pulled %d file(s) across %d project(s).\n", res.files, res.projects)
	return nil
}

func cmdPush(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	res, err := push(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Pushed %d file(s) across %d project(s).\n", res.files, res.projects)
	return nil
}

func cmdStatus(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	fmt.Printf("device:    %s (%s)\n", cfg.Device, platform())
	fmt.Printf("data repo: %s\n", cfg.RepoURL)
	fmt.Printf("claude:    %s\n", cfg.ClaudeDir)
	fmt.Printf("include:   %v\n", cfg.Include)
	fmt.Printf("exclude:   %v\n", cfg.Exclude)

	projects := filepath.Join(cfg.ClaudeDir, projectsSubdir)
	entries, err := os.ReadDir(projects)
	if err != nil {
		return nil
	}
	var synced, skipped []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if matchFilter(e.Name(), cfg.Include, cfg.Exclude) {
			synced = append(synced, e.Name())
		} else {
			skipped = append(skipped, e.Name())
		}
	}
	fmt.Printf("\nprojects synced (%d):\n", len(synced))
	for _, n := range synced {
		fmt.Printf("  + %s\n", n)
	}
	fmt.Printf("projects skipped (%d):\n", len(skipped))
	for _, n := range skipped {
		fmt.Printf("  - %s\n", n)
	}

	if isGitRepo(cfg.WorkDir) {
		if reg, err := loadRegistry(cfg.WorkDir); err == nil {
			fmt.Printf("\ndevice chain (%d):\n", len(reg.Devices))
			for _, d := range reg.Devices {
				fmt.Printf("  • %-20s last sync %s\n", d.Name, d.LastSync)
			}
		}
	}
	return nil
}

func cmdDevice(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	if err := ensureRepo(cfg); err != nil {
		return err
	}
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	reg, err := loadRegistry(cfg.WorkDir)
	if err != nil {
		return err
	}
	switch sub {
	case "list":
		fmt.Printf("device chain (%d):\n", len(reg.Devices))
		for _, d := range reg.Devices {
			marker := " "
			if d.Name == cfg.Device {
				marker = "*"
			}
			fmt.Printf(" %s %-20s %-12s added %s  last sync %s\n", marker, d.Name, d.Platform, d.AddedAt, d.LastSync)
		}
		return nil
	case "remove":
		if len(args) < 2 {
			return fmt.Errorf("usage: ccsync device remove <name>")
		}
		if !reg.remove(args[1]) {
			return fmt.Errorf("device %q not found in chain", args[1])
		}
		if err := reg.save(cfg.WorkDir); err != nil {
			return err
		}
		if _, err := git(cfg.WorkDir, "add", "devices.json"); err != nil {
			return err
		}
		if _, err := git(cfg.WorkDir, "commit", "-m", fmt.Sprintf("device: remove %s", args[1])); err != nil {
			return err
		}
		if err := gitStream(cfg.WorkDir, "push"); err != nil {
			return err
		}
		fmt.Printf("Removed %q from the chain.\n", args[1])
		return nil
	default:
		return fmt.Errorf("unknown device subcommand %q (list|remove)", sub)
	}
}

func cmdFilter(args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	sub := "list"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "list":
		fmt.Printf("include: %v\nexclude: %v\n", cfg.Include, cfg.Exclude)
		return nil
	case "add", "remove":
		fs := flag.NewFlagSet("filter", flag.ExitOnError)
		inc := fs.String("include", "", "include glob")
		exc := fs.String("exclude", "", "exclude glob")
		_ = fs.Parse(args[1:])
		if *inc == "" && *exc == "" {
			return fmt.Errorf("pass --include <glob> or --exclude <glob>")
		}
		if *inc != "" {
			cfg.Include = mutateList(cfg.Include, *inc, sub == "add")
		}
		if *exc != "" {
			cfg.Exclude = mutateList(cfg.Exclude, *exc, sub == "add")
		}
		if err := cfg.save(); err != nil {
			return err
		}
		fmt.Printf("include: %v\nexclude: %v\n", cfg.Include, cfg.Exclude)
		return nil
	default:
		return fmt.Errorf("unknown filter subcommand %q (list|add|remove)", sub)
	}
}

func mutateList(list []string, val string, add bool) []string {
	out := make([]string, 0, len(list))
	found := false
	for _, v := range list {
		if v == val {
			found = true
			if !add {
				continue // drop it
			}
		}
		out = append(out, v)
	}
	if add && !found {
		out = append(out, val)
	}
	return out
}
