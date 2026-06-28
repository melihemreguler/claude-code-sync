package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/claudefs"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/nocrypto"
	"github.com/melihemreguler/claude-code-sync/internal/app"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/domain"
)

// fakeIdent maps working directories to canonical keys without touching git.
type fakeIdent struct {
	keys map[string]domain.CanonicalKey
}

func (f fakeIdent) Key(cwd string) (domain.CanonicalKey, string) {
	if k, ok := f.keys[cwd]; ok {
		return k, filepath.Base(string(k))
	}
	return "", ""
}

// fakeStorage is a no-op backend over a shared local directory (no git).
type fakeStorage struct{ root string }

func (s *fakeStorage) EnsureLocal() error              { return os.MkdirAll(s.root, 0o755) }
func (s *fakeStorage) RemoteHasContent() (bool, error) { return false, nil }
func (s *fakeStorage) Pull() error                     { return nil }
func (s *fakeStorage) Push(string) error               { return nil }
func (s *fakeStorage) RootDir() string                 { return s.root }

func writeSession(t *testing.T, claudeDir, cwd, file string) {
	t.Helper()
	folder := domain.EncodeCwd(cwd)
	dir := filepath.Join(claudeDir, "projects", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"type":"user","cwd":"` + cwd + `","sessionId":"x"}` + "\n"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newSyncer(t *testing.T, device, claudeDir, home string, key domain.CanonicalKey, cwd string, include []string, root string) *app.Syncer {
	cfg := &config.Config{Device: device, ClaudeDir: claudeDir, Include: include}
	ident := fakeIdent{keys: map[string]domain.CanonicalKey{cwd: key}}
	return app.NewWith(cfg, claudefs.New(claudeDir), ident, &fakeStorage{root: root}, nocrypto.Passthrough{}, home)
}

// The P1 promise: the same logical project at different paths on two devices
// cross-syncs into each device's own path-encoded folder.
func TestCrossDeviceTranslation(t *testing.T) {
	root := t.TempDir()
	key := domain.CanonicalKey("github.com/acme/widgets")

	// Device A: project under ~/dev/github/widgets.
	claudeA := t.TempDir()
	cwdA := "/Users/a/dev/github/widgets"
	writeSession(t, claudeA, cwdA, "sessA.jsonl")
	sA := newSyncer(t, "A", claudeA, "/Users/a", key, cwdA, []string{"/Users/a/dev/github"}, root)
	if _, err := sA.Push(); err != nil {
		t.Fatalf("A push: %v", err)
	}

	// Device B: SAME project under ~/github/widgets (no dev), with its own session.
	claudeB := t.TempDir()
	cwdB := "/Users/b/github/widgets"
	writeSession(t, claudeB, cwdB, "sessB.jsonl")
	sB := newSyncer(t, "B", claudeB, "/Users/b", key, cwdB, []string{"/Users/b/github"}, root)
	if _, err := sB.Pull(); err != nil {
		t.Fatalf("B pull: %v", err)
	}

	got := filepath.Join(claudeB, "projects", domain.EncodeCwd(cwdB), "sessA.jsonl")
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("A's session was not translated into B's folder: %v", err)
	}
}

// Projects outside the include roots must never reach storage.
func TestPushRespectsIncludeRoots(t *testing.T) {
	root := t.TempDir()
	claudeA := t.TempDir()
	keep := "/Users/a/dev/github/widgets"
	skip := "/Users/a/work/secret"
	writeSession(t, claudeA, keep, "keep.jsonl")
	writeSession(t, claudeA, skip, "secret.jsonl")

	cfg := &config.Config{Device: "A", ClaudeDir: claudeA, Include: []string{"/Users/a/dev/github"}}
	ident := fakeIdent{keys: map[string]domain.CanonicalKey{
		keep: "github.com/acme/widgets",
		skip: "github.com/acme/secret",
	}}
	s := app.NewWith(cfg, claudefs.New(claudeA), ident, &fakeStorage{root: root}, nocrypto.Passthrough{}, "/Users/a")
	if _, err := s.Push(); err != nil {
		t.Fatal(err)
	}

	objects := filepath.Join(root, "objects")
	if !dirHasFile(t, filepath.Join(objects, domain.KeyHash("github.com/acme/widgets")), "keep.jsonl") {
		t.Error("included project should have been pushed")
	}
	if _, err := os.Stat(filepath.Join(objects, domain.KeyHash("github.com/acme/secret"))); !os.IsNotExist(err) {
		t.Error("excluded project must not reach storage")
	}
}

func dirHasFile(t *testing.T, dir, name string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}
