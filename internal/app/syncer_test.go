package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/agecrypto"
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
func (s *fakeStorage) Delete(rel string) (bool, error) {
	p := filepath.Join(s.root, filepath.FromSlash(rel))
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, os.Remove(p)
}

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
	s := app.NewWith(cfg, claudefs.New(claudeDir), ident, &fakeStorage{root: root}, nocrypto.Passthrough{}, home)
	s.SetLockDir(t.TempDir()) // hermetic: don't share the per-user sync.lock with a background ccsync
	return s
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

// Each device writes its own manifest shard; the merged view unions them.
func TestPerDeviceShards(t *testing.T) {
	root := t.TempDir()

	claudeA := t.TempDir()
	cwdA := "/Users/a/dev/github/x"
	writeSession(t, claudeA, cwdA, "a.jsonl")
	sA := newSyncer(t, "A", claudeA, "/Users/a", "github.com/acme/x", cwdA, []string{"/Users/a/dev/github"}, root)
	if _, err := sA.Push(); err != nil {
		t.Fatal(err)
	}

	claudeB := t.TempDir()
	cwdB := "/Users/b/dev/github/y"
	writeSession(t, claudeB, cwdB, "b.jsonl")
	sB := newSyncer(t, "B", claudeB, "/Users/b", "github.com/acme/y", cwdB, []string{"/Users/b/dev/github"}, root)
	if _, err := sB.Push(); err != nil {
		t.Fatal(err)
	}

	for _, d := range []string{"A", "B"} {
		if _, err := os.Stat(filepath.Join(root, "manifests", d+".age")); err != nil {
			t.Errorf("missing shard for device %s: %v", d, err)
		}
	}
	m, err := sA.Manifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Devices) != 2 {
		t.Errorf("merged view should have 2 devices, got %d", len(m.Devices))
	}
	if len(m.Projects) != 2 {
		t.Errorf("merged view should have 2 projects, got %d", len(m.Projects))
	}
}

// import --all materializes a project the device has never opened locally, under
// the originating device's folder name; plain Pull leaves it alone.
func TestImportMaterializesAbsentProjects(t *testing.T) {
	root := t.TempDir()
	key := domain.CanonicalKey("github.com/acme/widgets")

	// Device B owns the project and pushes it to the chain.
	claudeB := t.TempDir()
	cwdB := "/Users/b/dev/github/widgets"
	writeSession(t, claudeB, cwdB, "sessB.jsonl")
	sB := newSyncer(t, "B", claudeB, "/Users/b", key, cwdB, []string{"/Users/b/dev/github"}, root)
	if _, err := sB.Push(); err != nil {
		t.Fatal(err)
	}
	folderB := domain.EncodeCwd(cwdB)

	// Device A has no local presence of it.
	claudeA := t.TempDir()
	sA := newSyncer(t, "A", claudeA, "/Users/a", "", "", []string{"/Users/a/github"}, root)

	if res, err := sA.Pull(); err != nil || res.Files != 0 {
		t.Fatalf("plain Pull should import nothing, got %d (err %v)", res.Files, err)
	}
	res, err := sA.Import()
	if err != nil {
		t.Fatal(err)
	}
	if res.Files == 0 {
		t.Fatal("Import should have materialized the absent project")
	}
	if _, err := os.Stat(filepath.Join(claudeA, "projects", folderB, "sessB.jsonl")); err != nil {
		t.Fatalf("imported session not found under origin folder %q: %v", folderB, err)
	}
}

// RemoveDevice deletes that device's shard via the Storage port and reports
// whether anything was removed.
func TestRemoveDeviceDeletesShard(t *testing.T) {
	root := t.TempDir()
	claudeA := t.TempDir()
	cwdA := "/Users/a/dev/github/x"
	writeSession(t, claudeA, cwdA, "a.jsonl")
	sA := newSyncer(t, "A", claudeA, "/Users/a", "github.com/acme/x", cwdA, []string{"/Users/a/dev/github"}, root)
	if _, err := sA.Push(); err != nil {
		t.Fatal(err)
	}
	shard := filepath.Join(root, "manifests", "A.age")
	if _, err := os.Stat(shard); err != nil {
		t.Fatalf("shard should exist after push: %v", err)
	}

	removed, err := sA.RemoveDevice("A")
	if err != nil || !removed {
		t.Fatalf("RemoveDevice: removed=%v err=%v", removed, err)
	}
	if _, err := os.Stat(shard); !os.IsNotExist(err) {
		t.Fatal("shard should be deleted after RemoveDevice")
	}

	removed, err = sA.RemoveDevice("ghost")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("removing a non-existent device should report false")
	}
}

// A session continued differently on two devices must merge into the record
// union on pull, rather than one device's tail clobbering the other's.
func TestPullMergesDivergentSessions(t *testing.T) {
	root := t.TempDir()
	key := domain.CanonicalKey("github.com/acme/widgets")

	// Device A: session s.jsonl with records r1, r2, r3.
	claudeA := t.TempDir()
	cwdA := "/Users/a/dev/github/widgets"
	folderA := domain.EncodeCwd(cwdA)
	writeSessionLines(t, claudeA, folderA, "s.jsonl",
		sessionRec("r1", cwdA), sessionRec("r2", cwdA), sessionRec("r3", cwdA))
	sA := newSyncer(t, "A", claudeA, "/Users/a", key, cwdA, []string{"/Users/a/dev/github"}, root)
	if _, err := sA.Push(); err != nil {
		t.Fatal(err)
	}

	// Device B: SAME session (same key) but with a divergent tail r4.
	claudeB := t.TempDir()
	cwdB := "/Users/b/dev/github/widgets"
	folderB := domain.EncodeCwd(cwdB)
	writeSessionLines(t, claudeB, folderB, "s.jsonl",
		sessionRec("r1", cwdB), sessionRec("r2", cwdB), sessionRec("r4", cwdB))
	sB := newSyncer(t, "B", claudeB, "/Users/b", key, cwdB, []string{"/Users/b/dev/github"}, root)

	if _, err := sB.Pull(); err != nil {
		t.Fatal(err)
	}

	merged, err := os.ReadFile(filepath.Join(claudeB, "projects", folderB, "s.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"uuid":"r1"`, `"uuid":"r2"`, `"uuid":"r3"`, `"uuid":"r4"`} {
		if !bytes.Contains(merged, []byte(want)) {
			t.Errorf("merged session missing %s:\n%s", want, merged)
		}
	}
}

func sessionRec(uuid, cwd string) string {
	return `{"uuid":"` + uuid + `","cwd":"` + cwd + `","type":"user"}`
}

func writeSessionLines(t *testing.T, claudeDir, folder, file string, recs ...string) {
	t.Helper()
	dir := filepath.Join(claudeDir, "projects", folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Join(recs, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// GC removes object blobs that no manifest references, keeping live ones.
func TestGCRemovesOrphans(t *testing.T) {
	root := t.TempDir()
	claudeA := t.TempDir()
	cwdA := "/Users/a/dev/github/x"
	writeSession(t, claudeA, cwdA, "a.jsonl")
	sA := newSyncer(t, "A", claudeA, "/Users/a", "github.com/acme/x", cwdA, []string{"/Users/a/dev/github"}, root)
	if _, err := sA.Push(); err != nil {
		t.Fatal(err)
	}
	liveObj := filepath.Join(root, "objects", domain.KeyHash("github.com/acme/x"), "a.jsonl.age")
	if _, err := os.Stat(liveObj); err != nil {
		t.Fatalf("live object should exist after push: %v", err)
	}

	// Plant an orphan blob under a key no manifest references.
	orphan := filepath.Join(root, "objects", "deadbeef", "gone.jsonl.age")
	if err := os.MkdirAll(filepath.Dir(orphan), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(orphan, []byte("orphaned ciphertext"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Dry run must not delete.
	res, err := sA.GC(true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Orphans != 1 || res.Freed == 0 {
		t.Fatalf("dry run should find 1 orphan with non-zero bytes, got %+v", res)
	}
	if _, err := os.Stat(orphan); err != nil {
		t.Fatal("dry run must not delete the orphan")
	}

	// Real run deletes the orphan, keeps the live object.
	res, err = sA.GC(false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Orphans != 1 {
		t.Fatalf("expected 1 orphan removed, got %d", res.Orphans)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Error("orphan should be deleted")
	}
	if _, err := os.Stat(liveObj); err != nil {
		t.Errorf("live object must survive GC: %v", err)
	}
}

// Sessions and the manifest must be ciphertext at rest in storage.
func TestObjectsEncryptedAtRest(t *testing.T) {
	root := t.TempDir()
	claudeDir := t.TempDir()
	cwd := "/Users/me/dev/github/widgets"
	secret := "TOP-SECRET-TOKEN-12345"
	writeSession(t, claudeDir, cwd, "s.jsonl")
	// overwrite with secret content
	folder := domain.EncodeCwd(cwd)
	if err := os.WriteFile(filepath.Join(claudeDir, "projects", folder, "s.jsonl"),
		[]byte(`{"cwd":"`+cwd+`","secret":"`+secret+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	idStr, _, _ := agecrypto.Generate()
	crypto, _ := agecrypto.New(idStr)
	cfg := &config.Config{Device: "A", ClaudeDir: claudeDir, Include: []string{"/Users/me/dev/github"}}
	ident := fakeIdent{keys: map[string]domain.CanonicalKey{cwd: "github.com/acme/widgets"}}
	s := app.NewWith(cfg, claudefs.New(claudeDir), ident, &fakeStorage{root: root}, crypto, "/Users/me")
	s.SetLockDir(t.TempDir())
	if _, err := s.Push(); err != nil {
		t.Fatal(err)
	}

	walkAssertNoSecret(t, root, secret)
}

func walkAssertNoSecret(t *testing.T, root, secret string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("plaintext secret leaked into %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// EnsureReady must reject a backend pointed at a non-ccsync repo (the cause of
// an early footgun: pointing --repo at the source repo instead of a data repo).
func TestEnsureReadyRejectsForeignRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := app.NewWith(&config.Config{Device: "A", ClaudeDir: t.TempDir()},
		claudefs.New(t.TempDir()), fakeIdent{}, &fakeStorage{root: root}, nocrypto.Passthrough{}, "/Users/me")
	if err := s.EnsureReady(); err == nil {
		t.Fatal("expected EnsureReady to reject a foreign repo")
	}
}

func TestEnsureReadyAcceptsCleanRepo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("tap data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := app.NewWith(&config.Config{Device: "A", ClaudeDir: t.TempDir()},
		claudefs.New(t.TempDir()), fakeIdent{}, &fakeStorage{root: root}, nocrypto.Passthrough{}, "/Users/me")
	if err := s.EnsureReady(); err != nil {
		t.Fatalf("clean repo should be accepted: %v", err)
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
	s.SetLockDir(t.TempDir())
	if _, err := s.Push(); err != nil {
		t.Fatal(err)
	}

	objects := filepath.Join(root, "objects")
	if _, err := os.Stat(filepath.Join(objects, domain.KeyHash("github.com/acme/widgets"), "keep.jsonl.age")); err != nil {
		t.Errorf("included project should have been pushed as an encrypted object: %v", err)
	}
	if _, err := os.Stat(filepath.Join(objects, domain.KeyHash("github.com/acme/secret"))); !os.IsNotExist(err) {
		t.Error("excluded project must not reach storage")
	}
}
