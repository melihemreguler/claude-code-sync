package cmd

import (
	"fmt"
	"os"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/spf13/cobra"
)

var importAll bool

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Materialize chain projects this device doesn't have locally",
	Long: `Pull the chain like 'sync', but also write to disk the projects this device
has never opened locally — using the originating device's folder name.

Because Claude Code keys sessions by absolute path, these imported sessions only
show up in 'claude --resume' from a matching working directory. The normal way to
bring a project's history to a new machine is to check the repo out and open it
once; 'import --all' is the escape hatch when you just want the files present.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if !importAll {
			return fmt.Errorf("pass --all to import projects not present on this device (they use the origin's folder name; see `ccsync import --help`)")
		}
		return withSyncer(func(s *app.Syncer) error {
			res, err := s.Import()
			if err != nil {
				return notBusy(err)
			}
			fmt.Printf("Imported %d file(s) across %d project(s).\n", res.Files, res.Projects)
			if res.Files > 0 {
				fmt.Fprintln(os.Stderr, "note: imported sessions use the originating device's folder name; `claude --resume` lists them only from a matching path.")
			}
			return nil
		})
	},
}

func init() {
	importCmd.Flags().BoolVar(&importAll, "all", false, "import projects not present on this device")
	rootCmd.AddCommand(importCmd)
}
