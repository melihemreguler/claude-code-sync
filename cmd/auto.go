package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/hookcfg"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/launchd"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/systemd"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

// agentSpec is the platform-neutral description of an auto-sync background agent.
// installAgent translates it into a launchd (macOS) or systemd (Linux) unit.
type agentSpec struct {
	label       string
	args        []string
	intervalSec int
	keepAlive   bool
	logPath     string
}

// installAgent installs a background agent using the OS-native scheduler.
func installAgent(exe string, spec agentSpec) error {
	switch runtime.GOOS {
	case "darwin":
		return launchd.Install(exe, launchd.Spec{
			Label: spec.label, Args: spec.args, IntervalSec: spec.intervalSec,
			KeepAlive: spec.keepAlive, LogPath: spec.logPath,
		})
	case "linux":
		return systemd.Install(exe, systemd.Spec{
			Label: spec.label, Args: spec.args, IntervalSec: spec.intervalSec,
			KeepAlive: spec.keepAlive, LogPath: spec.logPath,
		})
	default:
		return fmt.Errorf("background auto-sync agents are supported on macOS and Linux only "+
			"(this is %s); use --hooks, or run `ccsync watch` yourself", runtime.GOOS)
	}
}

// removeAgent removes a background agent installed by installAgent.
func removeAgent(label string) error {
	switch runtime.GOOS {
	case "darwin":
		return launchd.Remove(label)
	case "linux":
		return systemd.Remove(label)
	default:
		return nil
	}
}

// agentInstalled reports whether a background agent is installed on this OS.
func agentInstalled(label string) bool {
	switch runtime.GOOS {
	case "darwin":
		return launchd.Installed(label)
	case "linux":
		return systemd.Installed(label)
	default:
		return false
	}
}

const (
	syncAgentLabel  = "com.ccsync.sync"
	watchAgentLabel = "com.ccsync.watch"
)

var (
	autoHooks    bool
	autoLaunchd  bool
	autoWatch    bool
	autoInterval time.Duration

	disableHooks   bool
	disableLaunchd bool
	disableWatch   bool
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
	Short: "Disable auto-sync triggers (all, or only the ones you pass)",
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
	f.BoolVar(&autoLaunchd, "launchd", false, "sync periodically in the background (launchd on macOS, systemd timer on Linux)")
	f.BoolVar(&autoWatch, "watch", false, "sync in real time via a file watcher")
	f.DurationVar(&autoInterval, "interval", 15*time.Minute, "interval for --launchd")

	d := autoDisableCmd.Flags()
	d.BoolVar(&disableHooks, "hooks", false, "disable only the hooks trigger")
	d.BoolVar(&disableLaunchd, "launchd", false, "disable only the periodic trigger")
	d.BoolVar(&disableWatch, "watch", false, "disable only the watcher trigger")

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
		spec := agentSpec{label: syncAgentLabel, args: []string{"sync"}, intervalSec: int(interval.Seconds()), logPath: logPath}
		if err := installAgent(exe, spec); err != nil {
			return err
		}
		cfg.AutoLaunchd = true
		cfg.AutoIntervalSec = int(interval.Seconds())
		fmt.Printf("enabled: periodic sync every %s\n", interval)
	}
	if watch {
		spec := agentSpec{label: watchAgentLabel, args: []string{"watch"}, keepAlive: true, logPath: logPath}
		if err := installAgent(exe, spec); err != nil {
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
	// No flags → disable everything (back-compat). Any flag → only those.
	all := !disableHooks && !disableLaunchd && !disableWatch

	if all || disableHooks {
		if err := hookcfg.Remove(cfg.ClaudeDir); err != nil {
			return err
		}
		cfg.AutoHooks = false
	}
	if all || disableLaunchd {
		if err := removeAgent(syncAgentLabel); err != nil {
			return err
		}
		cfg.AutoLaunchd = false
	}
	if all || disableWatch {
		if err := removeAgent(watchAgentLabel); err != nil {
			return err
		}
		cfg.AutoWatch = false
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	if all {
		fmt.Println("disabled all auto-sync triggers")
	} else {
		fmt.Println("updated auto-sync triggers")
	}
	return nil
}

func runAutoStatus(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	fmt.Printf("hooks:    %v\n", cfg.AutoHooks)
	fmt.Printf("periodic: %v (installed: %v, interval: %ds)\n", cfg.AutoLaunchd, agentInstalled(syncAgentLabel), cfg.AutoIntervalSec)
	fmt.Printf("watcher:  %v (installed: %v)\n", cfg.AutoWatch, agentInstalled(watchAgentLabel))
	return nil
}

func autoLogPath() string {
	dir, err := config.Dir()
	if err != nil {
		return filepath.Join(os.TempDir(), "ccsync.log")
	}
	return filepath.Join(dir, "ccsync.log")
}
