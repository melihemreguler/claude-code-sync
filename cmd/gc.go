package cmd

import (
	"fmt"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/spf13/cobra"
)

var gcDryRun bool

var gcCmd = &cobra.Command{
	Use:   "gc",
	Short: "Remove stored session blobs no device references (e.g. after device remove)",
	Long: `Delete encrypted object blobs in storage that no device's manifest references —
typically the objects left behind when a device is removed from the chain. Sessions
still listed by any device are always kept; the manifest itself is never pruned.

Use --dry-run to preview what would be removed.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(func(s *app.Syncer) error {
			res, err := s.GC(gcDryRun)
			if err != nil {
				return notBusy(err)
			}
			verb := "Removed"
			if gcDryRun {
				verb = "Would remove"
			}
			fmt.Printf("%s %d orphaned object(s), %s %s.\n", verb, res.Orphans,
				map[bool]string{true: "freeing", false: "freed"}[gcDryRun], humanBytes(res.Freed))
			return nil
		})
	},
}

func init() {
	gcCmd.Flags().BoolVar(&gcDryRun, "dry-run", false, "report what would be removed without deleting")
	rootCmd.AddCommand(gcCmd)
}

// humanBytes renders a byte count in a compact human-readable form.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
