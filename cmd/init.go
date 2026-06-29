package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/agecrypto"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/ghcli"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/keychain"
	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/spf13/cobra"
)

var (
	initRepo         string
	initDevice       string
	initClaudeDir    string
	initWorkDir      string
	initInclude      []string
	initExclude      []string
	initNewChain     bool
	initJoin         bool
	initKey          string
	initBackend      string
	initCreateRepo   string
	initS3Bucket     string
	initS3Prefix     string
	initS3Region     string
	initGDriveFolder string
	initGDriveCreds  string
	initGDriveToken  string
	initNoInput      bool
	initClaudeBase   bool
	initAllowPublic  bool
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
	f.StringVar(&initDevice, "device", "", "device name (default: hostname)")
	f.BoolVar(&initNewChain, "new-chain", false, "generate a new encrypted chain")
	f.BoolVar(&initJoin, "join", false, "join an existing chain (needs its identity)")
	f.StringVar(&initKey, "key", "", "chain identity to join with (or read from stdin)")
	f.StringSliceVar(&initInclude, "include", nil, "directory roots to sync (repeatable)")
	f.StringSliceVar(&initExclude, "exclude", nil, "directory roots to keep local (repeatable)")
	f.StringVar(&initClaudeDir, "claude-dir", config.DefaultClaudeDir(), "Claude Code home directory")
	f.StringVar(&initWorkDir, "work-dir", config.DefaultWorkDir(), "local working/mirror directory")

	// Backend selection.
	f.StringVar(&initBackend, "backend", "git", "storage backend: git, s3, or gdrive")
	// git
	f.StringVar(&initRepo, "repo", "", "git URL of the PRIVATE data repo (git backend)")
	f.StringVar(&initCreateRepo, "create-repo", "", "create a PRIVATE GitHub repo via gh and use it (git backend)")
	// s3
	f.StringVar(&initS3Bucket, "s3-bucket", "", "S3 bucket (s3 backend)")
	f.StringVar(&initS3Prefix, "s3-prefix", "ccsync", "S3 key prefix (s3 backend)")
	f.StringVar(&initS3Region, "s3-region", "", "S3 region (s3 backend; defaults to AWS config)")
	// gdrive
	f.StringVar(&initGDriveFolder, "gdrive-folder", "", "Google Drive folder ID (gdrive backend)")
	f.StringVar(&initGDriveCreds, "gdrive-credentials", "", "path to OAuth client secret JSON (gdrive backend)")
	f.StringVar(&initGDriveToken, "gdrive-token", "", "path to cache the OAuth token (gdrive backend)")
	f.BoolVar(&initNoInput, "no-input", false, "skip the interactive welcome tour; use flags only")
	f.BoolVar(&initClaudeBase, "claude-base", false, "on join, publish this machine's sessions without importing the chain's history first")
	f.BoolVar(&initAllowPublic, "allow-public", false, "allow a public data repo (otherwise init refuses/asks)")
}

func runInit(_ *cobra.Command, _ []string) error {
	if isInteractive() && !initNoInput {
		if err := runTour(); err != nil {
			return err
		}
	}

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
		Backend:   initBackend,
		ChainID:   chainID,
		ClaudeDir: initClaudeDir,
		WorkDir:   initWorkDir,
		Include:   resolveRoots(initInclude),
		Exclude:   resolveRoots(initExclude),
	}
	if err := configureBackend(cfg); err != nil {
		return err
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

	// Joining: fail fast if the identity can't decrypt the existing chain, rather
	// than proceeding with a wrong key and a confusing sync error later.
	if initJoin {
		if _, err := s.Manifest(); err != nil {
			return fmt.Errorf("could not read this chain with the provided identity — is the key correct?\n  %w", err)
		}
	}

	fmt.Printf("Initialized device %q.\n", cfg.Device)
	fmt.Printf("  backend:   %s (%s)\n", cfg.Backend, backendTarget(cfg))
	fmt.Printf("  chain:     %s\n", cfg.ChainID)
	fmt.Printf("  include:   %v\n", cfg.Include)
	fmt.Printf("  exclude:   %v\n", cfg.Exclude)
	if len(cfg.Include) == 0 {
		fmt.Fprintln(os.Stderr, "warning: include list is empty — nothing will sync. Add a path with `ccsync filter add --include <dir>`.")
	}
	fmt.Println("Running first sync …")
	if initJoin && initClaudeBase {
		out, err := s.Push()
		if err != nil {
			return err
		}
		fmt.Printf("Published %d file(s); the chain's other history was not imported.\n", out.Files)
	} else {
		in, out, err := s.Sync()
		if err != nil {
			return err
		}
		fmt.Printf("Synced: %d file(s) in, %d file(s) out.\n", in.Files, out.Files)
	}

	// Apply any auto-sync triggers chosen in the tour.
	if autoHooks || autoLaunchd || autoWatch {
		if err := applyAuto(cfg, autoHooks, autoLaunchd, autoWatch, autoInterval); err != nil {
			return err
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
	}
	return nil
}

// configureBackend validates backend-specific flags and fills cfg accordingly.
func configureBackend(cfg *config.Config) error {
	switch cfg.Backend {
	case "", "git":
		cfg.Backend = "git"
		switch {
		case initCreateRepo != "":
			url, err := ghcli.CreatePrivateRepo(initCreateRepo)
			if err != nil {
				return err
			}
			fmt.Printf("Created private repo: %s\n", url)
			cfg.RepoURL = url
		case initRepo != "":
			cfg.RepoURL = initRepo
			if err := confirmRepoVisibility(cfg.RepoURL); err != nil {
				return err
			}
		default:
			return fmt.Errorf("git backend needs --repo <url> or --create-repo <name>")
		}
	case "s3":
		if initS3Bucket == "" {
			return fmt.Errorf("s3 backend needs --s3-bucket")
		}
		cfg.S3Bucket, cfg.S3Prefix, cfg.S3Region = initS3Bucket, initS3Prefix, initS3Region
	case "gdrive":
		if initGDriveFolder == "" || initGDriveCreds == "" {
			return fmt.Errorf("gdrive backend needs --gdrive-folder and --gdrive-credentials")
		}
		token := initGDriveToken
		if token == "" {
			token = defaultGDriveTokenPath()
		}
		cfg.GDriveFolderID = initGDriveFolder
		cfg.GDriveCredentials = resolveRoot(initGDriveCreds)
		cfg.GDriveToken = resolveRoot(token)
	default:
		return fmt.Errorf("unknown backend %q (use git, s3, or gdrive)", cfg.Backend)
	}
	return nil
}

// confirmRepoVisibility refuses (or, interactively, asks about) a public data
// repo. Encrypted content is safe, but a public repo still leaks metadata.
func confirmRepoVisibility(url string) error {
	private, known := ghcli.IsPrivate(url)
	if !known || private {
		return nil
	}
	if initAllowPublic {
		fmt.Fprintln(os.Stderr, "warning: the data repo is PUBLIC (proceeding due to --allow-public)")
		return nil
	}
	if isInteractive() && !initNoInput {
		ok := false
		if err := huh.NewForm(huh.NewGroup(
			huh.NewConfirm().
				Title("This data repo is PUBLIC. Sessions are encrypted, but a public repo exposes metadata. Use it anyway?").
				Value(&ok),
		)).Run(); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("aborted: point --repo at a private repo")
		}
		return nil
	}
	return fmt.Errorf("data repo %s is PUBLIC; use a private repo or pass --allow-public", url)
}

func backendTarget(cfg *config.Config) string {
	switch cfg.Backend {
	case "s3":
		return "s3://" + cfg.S3Bucket + "/" + cfg.S3Prefix
	case "gdrive":
		return "drive folder " + cfg.GDriveFolderID
	default:
		return cfg.RepoURL
	}
}

func defaultGDriveTokenPath() string {
	dir, err := config.Dir()
	if err != nil {
		return "gdrive-token.json"
	}
	return dir + "/gdrive-token.json"
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
