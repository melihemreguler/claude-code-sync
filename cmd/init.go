package cmd

import (
	"fmt"
	"os"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/registry"
	"github.com/melihemreguler/claude-code-sync/internal/syncer"
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

Create a PRIVATE git repo to hold the session data and pass its URL with --repo.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	f := initCmd.Flags()
	f.StringVar(&initRepo, "repo", "", "git URL of the PRIVATE data repo (required)")
	f.StringVar(&initDevice, "device", "", "device name (default: hostname)")
	f.StringSliceVar(&initInclude, "include", []string{"*github*"}, "include globs (comma-separated)")
	f.StringSliceVar(&initExclude, "exclude", nil, "exclude globs (comma-separated)")
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

	s := syncer.New(cfg)
	if err := s.EnsureRepo(); err != nil {
		return err
	}
	if err := s.SeedRepo(); err != nil {
		return err
	}

	reg, err := registry.Load(cfg.WorkDir)
	if err != nil {
		return err
	}
	reg.Upsert(cfg.Device, config.Platform())
	if err := reg.Save(cfg.WorkDir); err != nil {
		return err
	}

	fmt.Printf("Initialized device %q.\n", cfg.Device)
	fmt.Printf("  data repo: %s\n", cfg.RepoURL)
	fmt.Printf("  include:   %v\n", cfg.Include)
	fmt.Printf("  exclude:   %v\n", cfg.Exclude)
	fmt.Println("Running first sync …")

	in, out, err := s.Sync()
	if err != nil {
		return err
	}
	fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
	return nil
}
