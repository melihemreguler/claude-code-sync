# Architecture

ccsync follows a hexagonal (ports & adapters) layout. The core business rules
know nothing about git, age, S3, the filesystem, or Cobra — those are adapters
behind interfaces, wired together in `app`.

```
cmd/                     Cobra CLI; wires adapters into app, no business logic
  init, sync, pull, push, status, device, filter, key, auto, watch, tour

internal/
  domain/                Pure rules, zero I/O, imports nothing internal
    encoding.go          cwd → Claude folder name (lossy: '/' and '.' → '-')
    identity.go          git remote → CanonicalKey; home-relative fallback key
    pathfilter.go        include/exclude by directory root (empty include = none)
    manifest.go          Manifest/Device/Project/ObjectMeta + KeyHash

  ports/                 Interfaces the core depends on
    ClaudeStore, Identifier, Storage, Crypto

  app/                   Use cases (the only place that knows domain + I/O)
    syncer.go            pull/push/sync, lock, manifest load/save, object I/O
    queries.go           status, device list/remove

  adapters/              Concrete implementations of the ports
    claudefs/            ClaudeStore over ~/.claude (+ cwd discovery)
    gitident/            Identifier via `git remote get-url`
    gitstore/            Storage as a git repo
    blobstore/           Storage as a Mirror over a BlobStore (S3/Drive)
    s3store/             BlobStore via aws-sdk-go-v2
    gdrivestore/         BlobStore via the Drive API
    agecrypto/           Crypto via age (X25519); keyed object-name hashing
    keychain/            chain identity in the OS keychain (go-keyring)
    nocrypto/            passthrough Crypto (tests/tooling)
    hookcfg/             Claude Code settings.json hook install/remove
    launchd/             macOS LaunchAgents (periodic sync + keep-alive watch)
    ghcli/               `gh repo create` for the private data repo

  fileutil/              atomic writes + content hashing
  gitutil/               thin git CLI wrapper
  config/                Viper-based per-machine config (CCSYNC_* env overrides)
```

## Dependency rule

`domain` ← `ports` ← (`app`, `adapters`) ← `cmd`. Domain imports nothing
internal. Adapters implement ports structurally (Go implicit interfaces) and
never call each other. `app.New` builds the default adapters; `app.NewWith`
injects fakes for tests.

## A sync, end to end

1. `cmd/sync` → `app.New(cfg)` wires Crypto (from keychain) + Storage (by backend).
2. `Syncer.Sync` takes a `flock` lock, then:
   - **pull**: `storage.Pull` (refresh) → decrypt the manifest → for each project,
     resolve *this device's* folder and decrypt newer objects into `~/.claude`.
   - **push**: for each included local project, encrypt changed session files into
     `objects/<HMAC(key)>/…`, update the manifest, `storage.Push`.

Why a Crypto port from the start: P1 shipped a passthrough so P2 could drop in age
without touching the core. Same pattern lets P3 add S3/Drive behind `Storage`.
