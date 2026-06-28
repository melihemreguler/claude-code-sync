package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// Config is the per-machine configuration, stored at ~/.config/ccsync/config.json.
// It is intentionally NOT synced — each device keeps its own (device name, paths).
type Config struct {
	// Device is this machine's unique name in the sync chain.
	Device string `json:"device"`
	// RepoURL is the git remote that holds the synced session data.
	// Keep this repo PRIVATE — it contains your conversation history.
	RepoURL string `json:"repoUrl"`
	// ClaudeDir is the Claude Code home (default ~/.claude).
	ClaudeDir string `json:"claudeDir"`
	// WorkDir is the local clone of the data repo.
	WorkDir string `json:"workDir"`
	// Include/Exclude are glob patterns matched against project folder names
	// under <ClaudeDir>/projects. Folder names embed the full project path,
	// e.g. "-Users-me-dev-github-foo", so "*github*" selects everything under
	// ~/dev/github while leaving work repos (e.g. "*turknet*") untouched.
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "ccsync"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func defaultClaudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

func defaultWorkDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".local", "share", "ccsync", "repo")
}

func loadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("not initialized — run `ccsync init` first")
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if c.ClaudeDir == "" {
		c.ClaudeDir = defaultClaudeDir()
	}
	if c.WorkDir == "" {
		c.WorkDir = defaultWorkDir()
	}
	return &c, nil
}

func (c *Config) save() error {
	dir, err := configDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.json")
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func platform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}
