package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/hookcfg"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/launchd"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

const (
	syncAgentLabel  = "com.ccsync.sync"
	watchAgentLabel = "com.ccsync.watch"
)

var (
	autoHooks    bool
	autoLaunchd  bool
	autoWatch    bool
	autoInterval time.Duration
)

var autoCmd = &cobra.Command{
	Use:   "auto",
	Short: "Configure automatic syncing (hooks, periodic, or watcher)",
	Long: `Enable hands-off syncing via any combination of triggers — Claude Code hooks
(sync when a session starts/ends), a periodic launchd job, or a real-time file
watcher. You choose which; all are optional.`,
}

var autoEnableCmd = &cobra.Command{
	Use:   "enable",
	Short: "Enable one or more auto-sync triggers",
	Args:  cobra.NoArgs,
	RunE:  runAutoEnable,
}

var autoDisableCmd = &cobra.Command{
	Use:   "disable",
	Short: "Disable all auto-sync triggers",
	Args:  cobra.NoArgs,
	RunE:  runAutoDisable,
}

var autoStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show which auto-sync triggers are active",
	Args:  cobra.NoArgs,
	RunE:  runAutoStatus,
}

func init() {
	f := autoEnableCmd.Flags()
	f.BoolVar(&autoHooks, "hooks", false, "sync on Claude Code session start/end")
	f.BoolVar(&autoLaunchd, "launchd", false, "sync periodically via a launchd job")
	f.BoolVar(&autoWatch, "watch", false, "sync in real time via a file watcher")
	f.DurationVar(&autoInterval, "interval", 15*time.Minute, "interval for --launchd")
	autoCmd.AddCommand(autoEnableCmd, autoDisableCmd, autoStatusCmd)
}

func runAutoEnable(_ *cobra.Command, _ []string) error {
	if !autoHooks && !autoLaunchd && !autoWatch {
		return fmt.Errorf("pick at least one of --hooks, --launchd, --watch")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := applyAuto(cfg, autoHooks, autoLaunchd, autoWatch, autoInterval); err != nil {
		return err
	}
	return config.Save(cfg)
}

// applyAuto installs the selected triggers and records them on cfg (the caller
// saves). Shared by `auto enable` and the init welcome tour.
func applyAuto(cfg *config.Config, hooks, periodic, watch bool, interval time.Duration) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logPath := autoLogPath()

	if hooks {
		if err := hookcfg.Install(cfg.ClaudeDir, exe); err != nil {
			return err
		}
		cfg.AutoHooks = true
		fmt.Println("enabled: Claude Code hooks (pull on session start, push on end)")
	}
	if periodic {
		spec := launchd.Spec{Label: syncAgentLabel, Args: []string{"sync"}, IntervalSec: int(interval.Seconds()), LogPath: logPath}
		if err := launchd.Install(exe, spec); err != nil {
			return err
		}
		cfg.AutoLaunchd = true
		cfg.AutoIntervalSec = int(interval.Seconds())
		fmt.Printf("enabled: periodic sync every %s\n", interval)
	}
	if watch {
		spec := launchd.Spec{Label: watchAgentLabel, Args: []string{"watch"}, KeepAlive: true, LogPath: logPath}
		if err := launchd.Install(exe, spec); err != nil {
			return err
		}
		cfg.AutoWatch = true
		fmt.Println("enabled: real-time file watcher")
	}
	return nil
}

func runAutoDisable(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := hookcfg.Remove(cfg.ClaudeDir); err != nil {
		return err
	}
	if err := launchd.Remove(syncAgentLabel); err != nil {
		return err
	}
	if err := launchd.Remove(watchAgentLabel); err != nil {
		return err
	}
	cfg.AutoHooks, cfg.AutoLaunchd, cfg.AutoWatch = false, false, false
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Println("disabled all auto-sync triggers")
	return nil
}

func runAutoStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Printf("hooks:    %v\n", cfg.AutoHooks)
	fmt.Printf("launchd:  %v (installed: %v, interval: %ds)\n", cfg.AutoLaunchd, launchd.Installed(syncAgentLabel), cfg.AutoIntervalSec)
	fmt.Printf("watcher:  %v (installed: %v)\n", cfg.AutoWatch, launchd.Installed(watchAgentLabel))
	return nil
}

func autoLogPath() string {
	dir, err := config.Dir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ccsync.log")
	}
	return filepath.Join(dir, "ccsync.log")
}
