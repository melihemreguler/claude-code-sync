package cmd

import (
	"fmt"

	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/spf13/cobra"
)

var deviceCmd = &cobra.Command{
	Use:   "device",
	Short: "Manage the device chain (the control panel)",
}

var deviceListCmd = &cobra.Command{
	Use:   "list",
	Short: "Show devices in the sync chain and the directories each one syncs",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		return withSyncer(func(s *app.Syncer) error {
			m, err := s.Manifest()
			if err != nil {
				return err
			}
			fmt.Printf("device chain (%d):\n", len(m.Devices))
			for _, d := range m.SortedDevices() {
				fmt.Printf(" • %-20s %-12s added %s  last sync %s\n", d.Name, d.Platform, d.AddedAt, d.LastSync)
				fmt.Printf("     include: %v\n", d.Include)
				fmt.Printf("     exclude: %v\n", d.Exclude)
			}
			return nil
		})
	},
}

var deviceRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Drop a device from the chain",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		return withSyncer(func(s *app.Syncer) error {
			removed, err := s.RemoveDevice(args[0])
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("device %q not found in chain", args[0])
			}
			fmt.Printf("Removed %q from the chain.\n", args[0])
			return nil
		})
	},
}

func init() {
	deviceCmd.AddCommand(deviceListCmd, deviceRemoveCmd)
}
