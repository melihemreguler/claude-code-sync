// Package cmd defines the ccsync command-line interface using Cobra.
package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "ccsync",
	Short: "Selective, multi-device sync for Claude Code sessions",
	Long: `ccsync mirrors Claude Code session history between your devices through a
git repository you control — but only the projects you choose.

Filters match Claude project folder names, which embed the full path
(e.g. "-Users-me-dev-github-foo"). The default include is "*github*".`,
	SilenceUsage:  true,
	SilenceErrors: false,
}

// SetVersion wires the build-time version into the root command.
func SetVersion(v string) {
	rootCmd.Version = v
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(initCmd, syncCmd, pullCmd, pushCmd, statusCmd, deviceCmd, filterCmd)
}
