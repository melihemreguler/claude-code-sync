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
	// RepoURL is the git remote holding the synced session data. Keep it PRIVATE.
	RepoURL string `mapstructure:"repoUrl" json:"repoUrl"`
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
	v.Set("repoUrl", c.RepoURL)
	v.Set("claudeDir", c.ClaudeDir)
	v.Set("workDir", c.WorkDir)
	v.Set("include", c.Include)
	v.Set("exclude", c.Exclude)

	return v.WriteConfigAs(filepath.Join(dir, "config.json"))
}
