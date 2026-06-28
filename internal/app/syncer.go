// Package app orchestrates ccsync's use cases on top of the ports, wiring the
// default adapters together. It is the only place that knows about both the
// domain rules and the concrete I/O.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/melihemreguler/claude-code-sync/internal/adapters/claudefs"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gitident"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gitstore"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/nocrypto"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/domain"
	"github.com/melihemreguler/claude-code-sync/internal/fileutil"
	"github.com/melihemreguler/claude-code-sync/internal/ports"
)

const (
	objectsDir   = "objects"
	manifestFile = "manifest"
)

// Result tallies what a sync direction touched.
type Result struct {
	Files    int
	Projects int
}

// Syncer carries the use cases. Construct it with New for production wiring, or
// NewWith to inject fakes in tests.
type Syncer struct {
	cfg     *config.Config
	store   ports.ClaudeStore
	ident   ports.Identifier
	storage ports.Storage
	crypto  ports.Crypto

	include []string // cleaned, absolute include roots
	exclude []string
}

// New wires the default adapters for cfg.
func New(cfg *config.Config) *Syncer {
	home, _ := os.UserHomeDir()
	return NewWith(cfg,
		claudefs.New(cfg.ClaudeDir),
		gitident.New(home),
		gitstore.New(cfg.RepoURL, cfg.WorkDir),
		nocrypto.Passthrough{},
		home,
	)
}

// NewWith builds a Syncer from explicit ports.
func NewWith(cfg *config.Config, store ports.ClaudeStore, ident ports.Identifier, storage ports.Storage, crypto ports.Crypto, home string) *Syncer {
	return &Syncer{
		cfg:     cfg,
		store:   store,
		ident:   ident,
		storage: storage,
		crypto:  crypto,
		include: cleanRoots(cfg.Include, home),
		exclude: cleanRoots(cfg.Exclude, home),
	}
}

func cleanRoots(roots []string, home string) []string {
	out := make([]string, 0, len(roots))
	for _, r := range roots {
		if r == "" {
			continue
		}
		out = append(out, domain.CleanRoot(r, home))
	}
	return out
}

// EnsureReady makes the storage available locally and seeds the expected layout.
func (s *Syncer) EnsureReady() error {
	if err := s.storage.EnsureLocal(); err != nil {
		return err
	}
	return s.seed()
}

// seed creates the objects directory and repo hygiene files if missing.
func (s *Syncer) seed() error {
	root := s.storage.RootDir()
	if err := os.MkdirAll(filepath.Join(root, objectsDir), 0o755); err != nil {
		return err
	}
	writeIfMissing(filepath.Join(root, objectsDir, ".gitkeep"), nil)
	writeIfMissing(filepath.Join(root, ".gitignore"), []byte("*"+fileutil.TmpSuffix+"\n.DS_Store\n"))
	writeIfMissing(filepath.Join(root, ".gitattributes"),
		[]byte("# Session logs are append-only — union-merge concurrent edits.\n*.jsonl merge=union\n"))
	return nil
}

func writeIfMissing(path string, content []byte) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, content, 0o644)
	}
}

// Sync pulls remote changes, then pushes local ones.
func (s *Syncer) Sync() (in Result, out Result, err error) {
	if in, err = s.Pull(); err != nil {
		return in, out, err
	}
	out, err = s.Push()
	return in, out, err
}

// Pull integrates remote sessions into the local Claude store, translating each
// logical project to this device's folder name.
func (s *Syncer) Pull() (Result, error) {
	if err := s.EnsureReady(); err != nil {
		return Result{}, err
	}
	if err := s.refresh(); err != nil {
		return Result{}, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return Result{}, err
	}
	localKeys, err := s.resolveLocalKeys()
	if err != nil {
		return Result{}, err
	}

	var res Result
	root := s.storage.RootDir()
	for keyStr, entry := range m.Projects {
		key := domain.CanonicalKey(keyStr)
		folder := s.localFolderFor(key, entry, localKeys)
		if folder == "" {
			continue
		}
		src := filepath.Join(root, objectsDir, domain.KeyHash(key))
		dst := s.store.ProjectPath(folder)
		n, err := fileutil.CopyTree(src, dst)
		if err != nil {
			return res, err
		}
		if n > 0 {
			res.Projects++
			res.Files += n
		}
	}
	return res, nil
}

// Push copies selected local sessions into storage under their canonical keys,
// records this device and its folder mappings in the manifest, and publishes.
func (s *Syncer) Push() (Result, error) {
	if err := s.EnsureReady(); err != nil {
		return Result{}, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return Result{}, err
	}
	m.UpsertDevice(s.cfg.Device, config.Platform(), s.include, s.exclude)

	var res Result
	root := s.storage.RootDir()
	folders, err := s.store.ListProjects()
	if err != nil {
		return res, err
	}
	for _, folder := range folders {
		cwd, err := s.store.ReadCwd(folder)
		if err != nil {
			return res, err
		}
		if cwd == "" || !domain.IncludeCwd(cwd, s.include, s.exclude) {
			continue
		}
		key, display := s.ident.Key(cwd)
		if key == "" {
			continue
		}
		src := s.store.ProjectPath(folder)
		dst := filepath.Join(root, objectsDir, domain.KeyHash(key))
		n, err := fileutil.CopyTree(src, dst)
		if err != nil {
			return res, err
		}
		m.RecordProject(key, display, s.cfg.Device, folder)
		if n > 0 {
			res.Projects++
			res.Files += n
		}
	}

	if err := s.saveManifest(m); err != nil {
		return res, err
	}
	msg := fmt.Sprintf("sync: %d file(s) from %s", res.Files, s.cfg.Device)
	if err := s.storage.Push(msg); err != nil {
		return res, err
	}
	return res, nil
}

// resolveLocalKeys maps each local project's canonical key to its folder name so
// pull can land remote projects in the right place on this device.
func (s *Syncer) resolveLocalKeys() (map[domain.CanonicalKey]string, error) {
	folders, err := s.store.ListProjects()
	if err != nil {
		return nil, err
	}
	out := map[domain.CanonicalKey]string{}
	for _, folder := range folders {
		cwd, err := s.store.ReadCwd(folder)
		if err != nil {
			return nil, err
		}
		if cwd == "" {
			continue
		}
		if key, _ := s.ident.Key(cwd); key != "" {
			out[key] = folder
		}
	}
	return out, nil
}

// localFolderFor picks this device's folder for a logical project: a previously
// recorded mapping wins, otherwise a project the device already has locally
// (matched by canonical key). It returns "" when this device has no presence of
// the project — in that case we do NOT materialize it, since we cannot know the
// correct path-encoded folder name until the user opens the project here. The
// data stays in storage and lands on the next sync once a local session exists.
func (s *Syncer) localFolderFor(key domain.CanonicalKey, entry domain.ProjectEntry, localKeys map[domain.CanonicalKey]string) string {
	if f, ok := entry.Folders[s.cfg.Device]; ok {
		return f
	}
	if f, ok := localKeys[key]; ok {
		return f
	}
	return ""
}

// refresh pulls remote changes if the remote has any content.
func (s *Syncer) refresh() error {
	has, err := s.storage.RemoteHasContent()
	if err != nil {
		return err
	}
	if !has {
		return nil
	}
	return s.storage.Pull()
}

func (s *Syncer) manifestPath() string {
	return filepath.Join(s.storage.RootDir(), manifestFile)
}

func (s *Syncer) loadManifest() (*domain.Manifest, error) {
	data, err := os.ReadFile(s.manifestPath())
	if os.IsNotExist(err) {
		return domain.NewManifest(), nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.crypto.Open(data)
	if err != nil {
		return nil, fmt.Errorf("opening manifest: %w", err)
	}
	var m domain.Manifest
	if err := json.Unmarshal(plain, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}
	if m.Projects == nil {
		m.Projects = map[string]domain.ProjectEntry{}
	}
	return &m, nil
}

func (s *Syncer) saveManifest(m *domain.Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	sealed, err := s.crypto.Seal(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("sealing manifest: %w", err)
	}
	return os.WriteFile(s.manifestPath(), sealed, 0o644)
}
