// Package ports declares the interfaces the application core depends on. Concrete
// implementations live in internal/adapters; the core never imports them
// directly, which keeps the business logic isolated and testable (hexagonal
// architecture).
package ports

import "github.com/melihemreguler/claude-code-sync/internal/domain"

// ClaudeStore is the local Claude Code session store (~/.claude).
type ClaudeStore interface {
	// ListProjects returns the project folder names under projects/.
	ListProjects() ([]string, error)
	// ProjectPath returns the absolute path to a project folder.
	ProjectPath(folder string) string
	// ReadCwd returns the true working directory recorded in a project's session
	// files, or "" if none could be determined.
	ReadCwd(folder string) (string, error)
}

// Identifier resolves a project's path-independent canonical key.
type Identifier interface {
	// Key returns the canonical key and a human-friendly display name for the
	// project rooted at cwd. cwd of "" yields an empty key.
	Key(cwd string) (domain.CanonicalKey, string)
}

// Storage is the remote sync backend (git today; S3/Drive later). It manages a
// local working directory that mirrors the remote.
type Storage interface {
	// EnsureLocal makes the working copy available locally (e.g. clone).
	EnsureLocal() error
	// RemoteHasContent reports whether the remote has anything to pull yet.
	RemoteHasContent() (bool, error)
	// Pull integrates remote changes into the working copy.
	Pull() error
	// Push commits and publishes local changes with the given message.
	Push(message string) error
	// RootDir is the local working directory.
	RootDir() string
}

// Crypto seals and opens payloads. The P1 implementation is a passthrough; age
// encryption replaces it in P2 without touching the core.
type Crypto interface {
	Seal(plaintext []byte) ([]byte, error)
	Open(ciphertext []byte) ([]byte, error)
}
