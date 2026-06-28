package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var (
	filterInclude string
	filterExclude string
)

var filterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Manage include/exclude path patterns",
}

var filterListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show include/exclude patterns",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		printFilters(cfg)
		return nil
	},
}

var filterAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add an include or exclude pattern",
	Args:  cobra.NoArgs,
	RunE:  func(c *cobra.Command, _ []string) error { return mutateFilter(true) },
}

var filterRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an include or exclude pattern",
	Args:  cobra.NoArgs,
	RunE:  func(c *cobra.Command, _ []string) error { return mutateFilter(false) },
}

func init() {
	for _, c := range []*cobra.Command{filterAddCmd, filterRemoveCmd} {
		c.Flags().StringVar(&filterInclude, "include", "", "include glob")
		c.Flags().StringVar(&filterExclude, "exclude", "", "exclude glob")
	}
	filterCmd.AddCommand(filterListCmd, filterAddCmd, filterRemoveCmd)
}

func mutateFilter(add bool) error {
	if filterInclude == "" && filterExclude == "" {
		return fmt.Errorf("pass --include <glob> or --exclude <glob>")
	}
	if add {
		for _, p := range []string{filterInclude, filterExclude} {
			if p == "" {
				continue
			}
			if _, err := filepath.Match(p, ""); err != nil {
				return fmt.Errorf("invalid glob %q: %w", p, err)
			}
		}
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if filterInclude != "" {
		cfg.Include = mutateList(cfg.Include, filterInclude, add)
	}
	if filterExclude != "" {
		cfg.Exclude = mutateList(cfg.Exclude, filterExclude, add)
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	if len(cfg.Include) == 0 {
		fmt.Fprintln(os.Stderr, "warning: include list is now empty — nothing will sync. Add a pattern (use \"*\" for everything).")
	}
	printFilters(cfg)
	return nil
}

func printFilters(cfg *config.Config) {
	fmt.Printf("include: %v\nexclude: %v\n", cfg.Include, cfg.Exclude)
}

// mutateList adds or removes val from list, treating it as a set.
func mutateList(list []string, val string, add bool) []string {
	out := make([]string, 0, len(list))
	found := false
	for _, v := range list {
		if v == val {
			found = true
			if !add {
				continue
			}
		}
		out = append(out, v)
	}
	if add && !found {
		out = append(out, val)
	}
	return out
}
