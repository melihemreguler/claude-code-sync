package cmd

import (
	"fmt"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show config, which projects sync/skip, and the device chain",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(runStatus)
	},
}

func runStatus(s *app.Syncer) error {
	fmt.Printf("include roots: %v\n", s.IncludeRoots())
	fmt.Printf("exclude roots: %v\n", s.ExcludeRoots())

	projects, err := s.LocalProjects()
	if err != nil {
		return err
	}
	var synced, skipped []app.ProjectStatus
	for _, p := range projects {
		if p.Included {
			synced = append(synced, p)
		} else {
			skipped = append(skipped, p)
		}
	}

	fmt.Printf("\nprojects synced (%d):\n", len(synced))
	for _, p := range synced {
		fmt.Printf("  + %s  [%s]\n", p.Cwd, p.Key)
	}
	fmt.Printf("projects skipped (%d):\n", len(skipped))
	for _, p := range skipped {
		where := p.Cwd
		if where == "" {
			where = p.Folder + " (no cwd found)"
		}
		fmt.Printf("  - %s\n", where)
	}

	m, err := s.Manifest()
	if err != nil {
		return nil // chain is best-effort in status
	}
	fmt.Printf("\ndevice chain (%d):\n", len(m.Devices))
	for _, d := range m.SortedDevices() {
		fmt.Printf("  • %-20s %-12s last sync %s\n", d.Name, d.Platform, d.LastSync)
		fmt.Printf("      include: %v\n", d.Include)
		fmt.Printf("      exclude: %v\n", d.Exclude)
	}
	return nil
}
