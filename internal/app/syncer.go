// Package app orchestrates ccsync's use cases on top of the ports, wiring the
// default adapters together. It is the only place that knows about both the
// domain rules and the concrete I/O.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/agecrypto"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/blobstore"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/claudefs"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gdrivestore"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gitident"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/gitstore"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/keychain"
	"github.com/melihemreguler/claude-code-sync/internal/adapters/s3store"
	"github.com/melihemreguler/claude-code-sync/internal/config"
	"github.com/melihemreguler/claude-code-sync/internal/domain"
	"github.com/melihemreguler/claude-code-sync/internal/fileutil"
	"github.com/melihemreguler/claude-code-sync/internal/ports"
)

const (
	objectsDir   = "objects"
	manifestFile = "manifest"
	objectExt    = ".age"
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

// New wires the default adapters for cfg, loading the chain encryption key and
// the configured storage backend.
func New(cfg *config.Config) (*Syncer, error) {
	home, _ := os.UserHomeDir()
	crypto, err := buildCrypto(cfg)
	if err != nil {
		return nil, err
	}
	storage, err := buildStorage(cfg)
	if err != nil {
		return nil, err
	}
	return NewWith(cfg,
		claudefs.New(cfg.ClaudeDir),
		gitident.New(home),
		storage,
		crypto,
		home,
	), nil
}

// buildStorage selects the storage backend from config.
func buildStorage(cfg *config.Config) (ports.Storage, error) {
	switch cfg.Backend {
	case "", "git":
		return gitstore.New(cfg.RepoURL, cfg.WorkDir), nil
	case "s3":
		blobs, err := s3store.New(context.Background(), cfg.S3Bucket, cfg.S3Prefix, cfg.S3Region)
		if err != nil {
			return nil, err
		}
		return blobstore.NewMirror(blobs, cfg.WorkDir), nil
	case "gdrive":
		blobs, err := gdrivestore.New(context.Background(), cfg.GDriveFolderID, cfg.GDriveCredentials, cfg.GDriveToken)
		if err != nil {
			return nil, err
		}
		return blobstore.NewMirror(blobs, cfg.WorkDir), nil
	default:
		return nil, fmt.Errorf("unknown backend %q (use git, s3, or gdrive)", cfg.Backend)
	}
}

// buildCrypto loads the chain identity from the CCSYNC_IDENTITY env override
// (headless/CI) or the OS keychain, and returns an age-backed Crypto.
func buildCrypto(cfg *config.Config) (ports.Crypto, error) {
	if id := os.Getenv("CCSYNC_IDENTITY"); id != "" {
		return agecrypto.New(id)
	}
	if cfg.ChainID == "" {
		return nil, fmt.Errorf("no encryption key configured; run `ccsync init` first")
	}
	id, err := keychain.Load(cfg.ChainID)
	if err != nil {
		return nil, fmt.Errorf("loading chain key %q from keychain: %w", cfg.ChainID, err)
	}
	return agecrypto.New(id)
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
	if err := s.validateRepo(); err != nil {
		return err
	}
	return s.seed()
}

// validateRepo guards against pointing the data backend at the wrong place (e.g.
// a project or the ccsync source repo instead of a dedicated data repo). It
// refuses if the storage root holds files that aren't part of a ccsync chain.
func (s *Syncer) validateRepo() error {
	entries, err := os.ReadDir(s.storage.RootDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	allowed := map[string]bool{
		".git": true, ".gitignore": true, ".gitattributes": true, ".DS_Store": true,
		"README.md": true, "LICENSE": true,
		manifestFile: true, objectsDir: true,
	}
	var foreign []string
	for _, e := range entries {
		if !allowed[e.Name()] {
			foreign = append(foreign, e.Name())
		}
	}
	if len(foreign) > 0 {
		return fmt.Errorf("%q does not look like a ccsync data repo (found %v).\n"+
			"Point the backend at a dedicated, private, empty repo for session data —\n"+
			"not your project or the ccsync source. Fix the URL, then remove %q and re-init",
			s.storage.RootDir(), foreign, s.storage.RootDir())
	}
	return nil
}

// seed ensures the objects directory exists. Backend-specific hygiene (e.g. a
// git .gitignore) is handled by the storage adapter's EnsureLocal.
func (s *Syncer) seed() error {
	return os.MkdirAll(filepath.Join(s.storage.RootDir(), objectsDir), 0o755)
}

// ErrSyncInProgress is returned when another sync holds the lock on this machine.
var ErrSyncInProgress = errors.New("another sync is in progress")

// withLock serializes mutating operations on this machine so concurrent triggers
// (hook, launchd, watcher) never run at once. It skips rather than queues.
func (s *Syncer) withLock(fn func() error) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	fl := flock.New(filepath.Join(dir, "sync.lock"))
	locked, err := fl.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return ErrSyncInProgress
	}
	defer func() { _ = fl.Unlock() }()
	return fn()
}

// Sync pulls remote changes, then pushes local ones.
func (s *Syncer) Sync() (in Result, out Result, err error) {
	err = s.withLock(func() error {
		var e error
		if in, e = s.pull(); e != nil {
			return e
		}
		out, e = s.push()
		return e
	})
	return in, out, err
}

// Pull integrates remote sessions into the local Claude store under the lock.
func (s *Syncer) Pull() (Result, error) {
	var res Result
	err := s.withLock(func() error {
		var e error
		res, e = s.pull()
		return e
	})
	return res, err
}

// Push sends local sessions to storage under the lock.
func (s *Syncer) Push() (Result, error) {
	var res Result
	err := s.withLock(func() error {
		var e error
		res, e = s.push()
		return e
	})
	return res, err
}

// pull integrates remote sessions into the local Claude store, decrypting each
// object and translating each logical project to this device's folder name.
func (s *Syncer) pull() (Result, error) {
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
	for keyStr, entry := range m.Projects {
		key := domain.CanonicalKey(keyStr)
		folder := s.localFolderFor(key, entry, localKeys)
		if folder == "" {
			continue
		}
		n, err := s.pullProject(key, entry, folder)
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

// pullProject decrypts each of a project's objects into this device's folder,
// skipping objects that are unchanged or whose local copy is newer.
func (s *Syncer) pullProject(key domain.CanonicalKey, entry domain.ProjectEntry, folder string) (int, error) {
	count := 0
	for rel, meta := range entry.Objects {
		localPath := filepath.Join(s.store.ProjectPath(folder), filepath.FromSlash(rel))
		if info, err := os.Stat(localPath); err == nil {
			if info.ModTime().UnixNano() == meta.MTime {
				continue // fast path: untouched since last sync
			}
			data, err := os.ReadFile(localPath)
			if err != nil {
				return count, err
			}
			if fileutil.HashBytes(data) == meta.Hash {
				continue // identical content
			}
			if info.ModTime().UnixNano() >= meta.MTime {
				continue // local copy is at least as new — keep it
			}
		}
		sealed, err := os.ReadFile(s.objectPath(key, rel))
		if err != nil {
			return count, err
		}
		plain, err := s.crypto.Open(sealed)
		if err != nil {
			return count, fmt.Errorf("decrypting %s: %w", rel, err)
		}
		if err := fileutil.WriteFileAtomic(localPath, plain, time.Unix(0, meta.MTime)); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// push encrypts selected local sessions into storage under their canonical keys,
// records this device and its folder mappings in the manifest, and publishes.
func (s *Syncer) push() (Result, error) {
	if err := s.EnsureReady(); err != nil {
		return Result{}, err
	}
	m, err := s.loadManifest()
	if err != nil {
		return Result{}, err
	}
	m.UpsertDevice(s.cfg.Device, config.Platform(), s.include, s.exclude)

	var res Result
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
		n, err := s.pushProject(key, folder, m)
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

// pushProject encrypts each changed local session file into storage and records
// its metadata, skipping files that are unchanged or older than what is stored.
func (s *Syncer) pushProject(key domain.CanonicalKey, folder string, m *domain.Manifest) (int, error) {
	count := 0
	projectDir := s.store.ProjectPath(folder)
	entry := m.Projects[string(key)]

	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || strings.HasSuffix(d.Name(), fileutil.TmpSuffix) {
			return nil
		}
		relOS, err := filepath.Rel(projectDir, path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)

		info, err := d.Info()
		if err != nil {
			return err
		}
		mtime := info.ModTime().UnixNano()
		// Fast path: an untouched file (mtime unchanged since last sync) needs no
		// read/hash. We set the local mtime on pull and record it on push, so an
		// equal mtime means the content is what storage already has.
		if meta, ok := entry.Objects[rel]; ok && mtime == meta.MTime {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		hash := fileutil.HashBytes(data)

		if meta, ok := entry.Objects[rel]; ok {
			if meta.Hash == hash {
				return nil // unchanged
			}
			if mtime <= meta.MTime {
				return nil // stored copy is newer — don't clobber
			}
		}

		sealed, err := s.crypto.Seal(data)
		if err != nil {
			return fmt.Errorf("encrypting %s: %w", rel, err)
		}
		if err := fileutil.WriteFileAtomic(s.objectPath(key, rel), sealed, time.Time{}); err != nil {
			return err
		}
		m.SetObject(key, rel, domain.ObjectMeta{Hash: hash, MTime: mtime})
		count++
		return nil
	})
	if os.IsNotExist(err) {
		return count, nil
	}
	return count, err
}

func (s *Syncer) objectPath(key domain.CanonicalKey, rel string) string {
	return filepath.Join(s.storage.RootDir(), objectsDir, s.crypto.HashName(string(key)), filepath.FromSlash(rel)+objectExt)
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
// the project — we then do NOT materialize it, since the correct path-encoded
// folder is unknown until the user opens the project here. The data stays in
// storage and lands on the next sync once a local session exists.
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
		return nil, fmt.Errorf("opening manifest (wrong key?): %w", err)
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
