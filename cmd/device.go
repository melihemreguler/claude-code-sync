package cmd

import (
	"fmt"

	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/gitutil"
	"github.com/melihemreguler/claude-code-sync/internal/registry"
	"github.com/melihemreguler/claude-code-sync/internal/syncer"
	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Manage the device chain (the control panel)",
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show devices in the sync chain",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		cfg, reg, err := loadRegistry()
		if err != nil {
			return err
		}
		fmt.Printf("device chain (%d):\n", len(reg.Devices))
		for _, d := range reg.Devices {
			marker := " "
			if d.Name == cfg.Device {
				marker = "*"
			}
			fmt.Printf(" %s %-20s %-12s added %s  last sync %s\n",
				marker, d.Name, d.Platform, d.AddedAt, d.LastSync)
		}
		return nil
	},
}

var deviceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Drop a device from the chain",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		cfg, reg, err := loadRegistry()
		if err != nil {
			return err
		}
		name := args[0]
		if !reg.Remove(name) {
			return fmt.Errorf("device %q not found in chain", name)
		}
		if err := reg.Save(cfg.WorkDir); err != nil {
			return err
		}
		if _, err := gitutil.Run(cfg.WorkDir, "add", registry.FileName); err != nil {
			return err
		}
		if _, err := gitutil.Run(cfg.WorkDir, "commit", "-m", fmt.Sprintf("device: remove %s", name)); err != nil {
			return err
		}
		if err := gitutil.Stream(cfg.WorkDir, "push"); err != nil {
			return err
		}
		fmt.Printf("Removed %q from the chain.\n", name)
		return nil
	},
}

func init() {
	deviceCmd.AddCommand(deviceListCmd, deviceRemoveCmd)
}

// loadRegistry loads the config and the synced device registry, ensuring the
// data repo is present locally first.
func loadRegistry() (*config.Config, *registry.Registry, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	if err := syncer.New(cfg).EnsureRepo(); err != nil {
		return nil, nil, err
	}
	reg, err := registry.Load(cfg.WorkDir)
	if err != nil {
		return nil, nil, err
	}
	return cfg, reg, nil
}
