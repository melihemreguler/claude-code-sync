package cmd

import (
	"fmt"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull remote changes, then push local ones",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		in, out, err := syncer.New(cfg).Sync()
		if err != nil {
			return err
		}
		fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
		return nil
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Apply remote sessions into ~/.claude",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		res, err := syncer.New(cfg).Pull()
		if err != nil {
			return err
		}
		fmt.Printf("Pulled %d file(s) across %d project(s).\n", res.Files, res.Projects)
		return nil
	},
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Send local sessions to the remote",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		res, err := syncer.New(cfg).Push()
		if err != nil {
			return err
		}
		fmt.Printf("Pushed %d file(s) across %d project(s).\n", res.Files, res.Projects)
		return nil
	},
}
