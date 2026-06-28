package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/gitutil"
	"github.com/melihemreguler/claude-code-sync/internal/registry"
	"github.com/melihemreguler/claude-code-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show config, which projects sync/skip, and the device chain",
	Args:  cobra.NoArgs,
	RunE:  runStatus,
}

func runStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Printf("device:    %s (%s)\n", cfg.Device, config.Platform())
	fmt.Printf("data repo: %s\n", cfg.RepoURL)
	fmt.Printf("claude:    %s\n", cfg.ClaudeDir)
	fmt.Printf("include:   %v\n", cfg.Include)
	fmt.Printf("exclude:   %v\n", cfg.Exclude)

	projects := filepath.Join(cfg.ClaudeDir, syncer.ProjectsSubdir)
	entries, err := os.ReadDir(projects)
	if err != nil {
		return nil // nothing to report if there are no projects yet
	}

	var synced, skipped []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if syncer.MatchFilter(e.Name(), cfg.Include, cfg.Exclude) {
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

	if gitutil.IsRepo(cfg.WorkDir) {
		if reg, err := registry.Load(cfg.WorkDir); err == nil {
			fmt.Printf("\ndevice chain (%d):\n", len(reg.Devices))
			for _, d := range reg.Devices {
				fmt.Printf("  • %-20s %-12s last sync %s\n", d.Name, d.Platform, d.LastSync)
			}
		}
	}
	return nil
}
