package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

// withSyncer loads config and runs fn with a wired Syncer.
func withSyncer(fn func(*app.Syncer) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, err := app.New(cfg)
	if err != nil {
		return err
	}
	return fn(s)
}

// notBusy turns an in-progress lock into a soft no-op, so auto-sync triggers that
// overlap don't surface as errors.
func notBusy(err error) error {
	if errors.Is(err, app.ErrSyncInProgress) {
		fmt.Fprintln(os.Stderr, "another sync is in progress — skipping")
		return nil
	}
	return err
}

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Pull remote changes, then push local ones",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(func(s *app.Syncer) error {
			in, out, err := s.Sync()
			if err != nil {
				return notBusy(err)
			}
			fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
			return nil
		})
	},
}

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Apply remote sessions into ~/.claude",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(func(s *app.Syncer) error {
			res, err := s.Pull()
			if err != nil {
				return notBusy(err)
			}
			fmt.Printf("Pulled %d file(s) across %d project(s).\n", res.Files, res.Projects)
			return nil
		})
	},
}

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Send local sessions to the remote",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(func(s *app.Syncer) error {
			res, err := s.Push()
			if err != nil {
				return notBusy(err)
			}
			fmt.Printf("Pushed %d file(s) across %d project(s).\n", res.Files, res.Projects)
			return nil
		})
	},
}
