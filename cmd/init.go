package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/agecrypto"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/keychain"
	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var (
	initRepo      string
	initDevice    string
	initClaudeDir string
	initWorkDir   string
	initInclude   []string
	initExclude   []string
	initNewChain  bool
	initJoin      bool
	initKey       string
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize this device and run the first sync",
	Long: `Initialize this device: set up the encryption key, write the local config,
clone the data repo, register the device, and run a first sync.

Start a new encrypted chain with --new-chain, or join an existing one with --join
(provide its identity via --key or stdin). The session data is end-to-end
encrypted; the chain identity is kept in your OS keychain, never in the repo.

Choose which projects sync by directory with --include / --exclude (paths, not
patterns). An empty include list syncs nothing.`,
	Args: cobra.NoArgs,
	RunE: runInit,
}

func init() {
	f := initCmd.Flags()
	f.StringVar(&initRepo, "repo", "", "git URL of the PRIVATE data repo (required)")
	f.StringVar(&initDevice, "device", "", "device name (default: hostname)")
	f.BoolVar(&initNewChain, "new-chain", false, "generate a new encrypted chain")
	f.BoolVar(&initJoin, "join", false, "join an existing chain (needs its identity)")
	f.StringVar(&initKey, "key", "", "chain identity to join with (or read from stdin)")
	f.StringSliceVar(&initInclude, "include", nil, "directory roots to sync (repeatable)")
	f.StringSliceVar(&initExclude, "exclude", nil, "directory roots to keep local (repeatable)")
	f.StringVar(&initClaudeDir, "claude-dir", config.DefaultClaudeDir(), "Claude Code home directory")
	f.StringVar(&initWorkDir, "work-dir", config.DefaultWorkDir(), "local clone location for the data repo")
	_ = initCmd.MarkFlagRequired("repo")
}

func runInit(_ *cobra.Command, _ []string) error {
	name := initDevice
	if name == "" {
		name, _ = os.Hostname()
	}
	if name == "" {
		return fmt.Errorf("could not determine device name; pass --device")
	}

	chainID, err := setupChainKey()
	if err != nil {
		return err
	}

	cfg := &config.Config{
		Device:    name,
		RepoURL:   initRepo,
		ChainID:   chainID,
		ClaudeDir: initClaudeDir,
		WorkDir:   initWorkDir,
		Include:   resolveRoots(initInclude),
		Exclude:   resolveRoots(initExclude),
	}
	if err := config.Save(cfg); err != nil {
		return err
	}

	s, err := app.New(cfg)
	if err != nil {
		return err
	}
	if err := s.EnsureReady(); err != nil {
		return err
	}

	fmt.Printf("Initialized device %q.\n", cfg.Device)
	fmt.Printf("  data repo: %s\n", cfg.RepoURL)
	fmt.Printf("  chain:     %s\n", cfg.ChainID)
	fmt.Printf("  include:   %v\n", cfg.Include)
	fmt.Printf("  exclude:   %v\n", cfg.Exclude)
	if len(cfg.Include) == 0 {
		fmt.Fprintln(os.Stderr, "warning: include list is empty — nothing will sync. Add a path with `ccsync filter add --include <dir>`.")
	}
	fmt.Println("Running first sync …")

	in, out, err := s.Sync()
	if err != nil {
		return err
	}
	fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
	return nil
}

// setupChainKey establishes this device's chain identity and returns the chain
// id (the age recipient). With CCSYNC_IDENTITY set, that identity is used and not
// written to the keychain; otherwise --new-chain generates one and --join imports
// one, both stored in the OS keychain.
func setupChainKey() (string, error) {
	if env := strings.TrimSpace(os.Getenv("CCSYNC_IDENTITY")); env != "" {
		return agecrypto.RecipientFromIdentity(env)
	}

	if initNewChain == initJoin { // both or neither
		return "", fmt.Errorf("choose exactly one of --new-chain or --join")
	}

	var identity string
	if initJoin {
		var err error
		identity, err = readIdentity()
		if err != nil {
			return "", err
		}
	} else {
		var recipient string
		var err error
		identity, recipient, err = agecrypto.Generate()
		if err != nil {
			return "", err
		}
		fmt.Println("Generated a new chain identity. Back this up and use it to join other devices:")
		fmt.Printf("\n  %s\n\n", identity)
		fmt.Printf("(public id: %s)\n", recipient)
	}

	recipient, err := agecrypto.RecipientFromIdentity(identity)
	if err != nil {
		return "", err
	}
	if err := keychain.Store(recipient, identity); err != nil {
		return "", fmt.Errorf("storing chain key in keychain: %w", err)
	}
	return recipient, nil
}

// readIdentity returns the join identity from --key or, if absent, stdin.
func readIdentity() (string, error) {
	if strings.TrimSpace(initKey) != "" {
		return strings.TrimSpace(initKey), nil
	}
	fmt.Print("Paste the chain identity (AGE-SECRET-KEY-1…): ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && strings.TrimSpace(line) == "" {
		return "", fmt.Errorf("no identity provided")
	}
	return strings.TrimSpace(line), nil
}
