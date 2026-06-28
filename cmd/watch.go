package cmd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var watchDebounce time.Duration

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch ~/.claude/projects and sync on change (real-time auto-sync)",
	Long: `Run a foreground watcher that syncs shortly after session files change.
Changes are debounced so a burst of writes triggers a single sync. Intended to be
kept alive by a process manager (see 'ccsync auto enable --watch').`,
	Args: cobra.NoArgs,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().DurationVar(&watchDebounce, "debounce", 10*time.Second, "quiet period before syncing after a change")
}

func runWatch(_ *cobra.Command, _ []string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	s, err := app.New(cfg)
	if err != nil {
		return err
	}
	root := filepath.Join(cfg.ClaudeDir, "projects")

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()
	if err := addRecursive(w, root); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(watchDebounce)
	defer ticker.Stop()
	dirty := false

	fmt.Printf("watching %s (debounce %s) — Ctrl-C to stop\n", root, watchDebounce)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-w.Events:
			if !ok {
				return nil
			}
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					_ = w.Add(ev.Name)
				}
			}
			dirty = true
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintln(os.Stderr, "watch error:", err)
		case <-ticker.C:
			if dirty {
				dirty = false
				syncQuiet(s)
			}
		}
	}
}

// syncQuiet runs a sync, ignoring the "another sync in progress" case and
// reporting only meaningful outcomes.
func syncQuiet(s *app.Syncer) {
	in, out, err := s.Sync()
	switch {
	case errors.Is(err, app.ErrSyncInProgress):
		return
	case err != nil:
		fmt.Fprintln(os.Stderr, "sync error:", err)
	case in.Files+out.Files > 0:
		fmt.Printf("synced: %d in, %d out\n", in.Files, out.Files)
	}
}

func addRecursive(w *fsnotify.Watcher, root string) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return w.Add(path)
		}
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
