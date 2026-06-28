package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/keychain"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Inspect the chain encryption key",
}

var keyShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print the chain identity (secret) to join another device",
	Long: `Print the chain's secret identity so you can join another device with
` + "`ccsync init --join`" + `. Treat this like a password — anyone with it can
decrypt the whole chain. Transfer it over a trusted channel only.`,
	Args: cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		if env := strings.TrimSpace(os.Getenv("CCSYNC_IDENTITY")); env != "" {
			fmt.Println(env)
			return nil
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.ChainID == "" {
			return fmt.Errorf("no chain configured; run `ccsync init` first")
		}
		identity, err := keychain.Load(cfg.ChainID)
		if err != nil {
			return fmt.Errorf("loading chain key from keychain: %w", err)
		}
		fmt.Fprintln(os.Stderr, "# WARNING: this is a secret. Anyone with it can decrypt your sessions.")
		fmt.Println(identity)
		return nil
	},
}

var keyIDCmd = &cobra.Command{
	Use:   "id",
	Short: "Print the chain's public id (age recipient)",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.ChainID == "" {
			return fmt.Errorf("no chain configured; run `ccsync init` first")
		}
		fmt.Println(cfg.ChainID)
		return nil
	},
}

func init() {
	keyCmd.AddCommand(keyShowCmd, keyIDCmd)
}
