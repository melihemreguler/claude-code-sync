// Package config loads and persists the per-machine ccsync configuration.
//
// The config is intentionally NOT synced between devices — each machine keeps
// its own device name, paths and filter set. Reading goes through Viper so that
// defaults and CCSYNC_* environment overrides are honored.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/viper"
)

// ErrNotInitialized is returned by Load when no config file exists yet.
var ErrNotInitialized = errors.New("not initialized — run `ccsync init` first")

// Config is the per-machine configuration stored at <UserConfigDir>/ccsync/config.json.
type Config struct {
	// Device is this machine's unique name in the sync chain.
	Device string `mapstructure:"device" json:"device"`
	// Backend selects the storage provider: "git" (default), "s3", or "gdrive".
	Backend string `mapstructure:"backend" json:"backend"`
	// RepoURL is the git remote holding the synced session data (git backend).
	// Keep it PRIVATE.
	RepoURL string `mapstructure:"repoUrl" json:"repoUrl"`
	// S3 backend settings.
	S3Bucket string `mapstructure:"s3Bucket" json:"s3Bucket,omitempty"`
	S3Prefix string `mapstructure:"s3Prefix" json:"s3Prefix,omitempty"`
	S3Region string `mapstructure:"s3Region" json:"s3Region,omitempty"`
	// Google Drive backend settings. CredentialsPath points to an OAuth client
	// secret JSON; TokenPath caches the user's authorization.
	GDriveFolderID    string `mapstructure:"gdriveFolderId" json:"gdriveFolderId,omitempty"`
	GDriveCredentials string `mapstructure:"gdriveCredentials" json:"gdriveCredentials,omitempty"`
	GDriveToken       string `mapstructure:"gdriveToken" json:"gdriveToken,omitempty"`
	// ChainID is the age recipient (public key) of the sync chain. It identifies
	// which secret identity to load from the keychain; it is not itself secret.
	ChainID string `mapstructure:"chainId" json:"chainId"`
	// ClaudeDir is the Claude Code home (default ~/.claude).
	ClaudeDir string `mapstructure:"claudeDir" json:"claudeDir"`
	// WorkDir is the local clone of the data repo.
	WorkDir string `mapstructure:"workDir" json:"workDir"`
	// Include/Exclude are directory roots (paths, not globs). A project is synced
	// when its working directory lies within an include root and within no
	// exclude root. E.g. include "~/dev/github" syncs everything under it while
	// excluding "~/dev/github/work" keeps those sessions local. An empty include
	// list syncs nothing.
	Include []string `mapstructure:"include" json:"include"`
	Exclude []string `mapstructure:"exclude" json:"exclude"`

	// Auto-sync trigger settings (P4). Each is opt-in; the user chooses which.
	AutoHooks       bool `mapstructure:"autoHooks" json:"autoHooks"`
	AutoLaunchd     bool `mapstructure:"autoLaunchd" json:"autoLaunchd"`
	AutoWatch       bool `mapstructure:"autoWatch" json:"autoWatch"`
	AutoIntervalSec int  `mapstructure:"autoIntervalSec" json:"autoIntervalSec"`
}

// Dir returns the ccsync config directory, creating nothing.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ccsync"), nil
}

// Path returns the absolute path to config.json.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// DefaultClaudeDir returns ~/.claude.
func DefaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// DefaultWorkDir returns the default local clone location for the data repo.
func DefaultWorkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "ccsync", "repo")
}

// Platform returns a short os/arch string for the device registry.
func Platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

// Load reads the configuration via Viper, applying defaults and CCSYNC_* env
// overrides. It returns ErrNotInitialized if the config file does not exist.
func Load() (*Config, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}

	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("json")
	v.AddConfigPath(dir)
	v.SetEnvPrefix("CCSYNC")
	v.AutomaticEnv()

	v.SetDefault("claudeDir", DefaultClaudeDir())
	v.SetDefault("workDir", DefaultWorkDir())
	v.SetDefault("include", []string{})
	v.SetDefault("exclude", []string{})

	if err := v.ReadInConfig(); err != nil {
		var notFound viper.ConfigFileNotFoundError
		if errors.As(err, &notFound) {
			return nil, ErrNotInitialized
		}
		return nil, err
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save writes the configuration to disk via Viper, creating the directory.
func Save(c *Config) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	v := viper.New()
	v.Set("device", c.Device)
	v.Set("backend", c.Backend)
	v.Set("repoUrl", c.RepoURL)
	v.Set("chainId", c.ChainID)
	v.Set("s3Bucket", c.S3Bucket)
	v.Set("s3Prefix", c.S3Prefix)
	v.Set("s3Region", c.S3Region)
	v.Set("gdriveFolderId", c.GDriveFolderID)
	v.Set("gdriveCredentials", c.GDriveCredentials)
	v.Set("gdriveToken", c.GDriveToken)
	v.Set("autoHooks", c.AutoHooks)
	v.Set("autoLaunchd", c.AutoLaunchd)
	v.Set("autoWatch", c.AutoWatch)
	v.Set("autoIntervalSec", c.AutoIntervalSec)
	v.Set("claudeDir", c.ClaudeDir)
	v.Set("workDir", c.WorkDir)
	v.Set("include", c.Include)
	v.Set("exclude", c.Exclude)

	return v.WriteConfigAs(filepath.Join(dir, "config.json"))
}
