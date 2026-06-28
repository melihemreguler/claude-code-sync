package cmd

import (
	"fmt"
	"os"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var (
	initRepo      string
	initDevice    string
	initClaudeDir string
	initWorkDir   string
	initInclude   []string
	initExclude   []string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize this device and run the first sync",
	Long: `Initialize this device: write the local config, clone the data repo, register
the device in the chain, and run a first sync.

Create a PRIVATE git repo to hold the session data and pass its URL with --repo.
Choose which projects sync by directory with --include / --exclude (paths, not
patterns). An empty include list syncs nothing.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	f := initCmd.Flags()
	f.StringVar(&initRepo, "repo", "", "git URL of the PRIVATE data repo (required)")
	f.StringVar(&initDevice, "device", "", "device name (default: hostname)")
	f.StringSliceVar(&initInclude, "include", nil, "directory roots to sync (repeatable)")
	f.StringSliceVar(&initExclude, "exclude", nil, "directory roots to keep local (repeatable)")
	f.StringVar(&initClaudeDir, "claude-dir", config.DefaultClaudeDir(), "Claude Code home directory")
	f.StringVar(&initWorkDir, "work-dir", config.DefaultWorkDir(), "local clone location for the data repo")
	_ = initCmd.MarkFlagRequired("repo")
}

func runInit(_ *cobra.Command, _ []string) error {
	name := initDevice
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" {
		return fmt.Errorf("could not determine device name; pass --device")
	}

	cfg := &config.Config{
		Device:    name,
		RepoURL:   initRepo,
		ClaudeDir: initClaudeDir,
		WorkDir:   initWorkDir,
		Include:   initInclude,
		Exclude:   initExclude,
	}
	if err := config.Save(cfg); err != nil {
		return err
	}

	s := app.New(cfg)
	if err := s.EnsureReady(); err != nil {
		return err
	}

	fmt.Printf("Initialized device %q.\n", cfg.Device)
	fmt.Printf("  data repo: %s\n", cfg.RepoURL)
	fmt.Printf("  include:   %v\n", cfg.Include)
	fmt.Printf("  exclude:   %v\n", cfg.Exclude)
	if len(cfg.Include) == 0 {
		fmt.Fprintln(os.Stderr, "warning: include list is empty — nothing will sync. Add a path with `ccsync filter add --include <dir>`.")
	}
	fmt.Println("Running first sync …")

	in, out, err := s.Sync()
	if err != nil {
		return err
	}
	fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
	return nil
}
