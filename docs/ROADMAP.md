# ccsync Roadmap & Architecture Plan

This document is the source of truth for where `ccsync` is headed and why. It is
written so a new contributor — human or AI agent — can onboard without prior
context. It complements the user-facing [README](../README.md).

## Vision

Zero-mental-overhead, **encrypted**, **provider-agnostic** sync of Claude Code
session history across a chain of devices, where each device chooses **which
projects** participate by path, and cross-device path differences are handled
transparently.

## Locked design decisions

These were decided deliberately; revisit only with a documented reason.

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | **Canonical-key + manifest** storage layout with per-device path translation | Decouples devices; sessions are keyed by a logical project id, not a machine-specific folder name. Never moves the user's code, never injects prompts into sessions. |
| D2 | **age encryption**, keys in the OS keychain; the sync core does decrypt → merge → encrypt | End-to-end secrecy even if the remote leaks. Git/cloud becomes a dumb ciphertext transport. The **manifest is encrypted too** (project paths are sensitive metadata). |
| D3 | **Provider-agnostic core with its own merge** (not git union-merge) | Required by D2 (can't merge ciphertext) and by D4 (S3/Drive have no merge). |
| D4 | **Strategy pattern for storage**: git (default), S3, Google Drive | User picks the backend; the core only sees a `Storage` port. |
| D5 | **Auto-sync via all of: Claude Code hooks, periodic launchd, fsnotify watcher** — user chooses in config | Different users want different trade-offs; all three are implemented, selection is config-driven. |
| D6 | **Hexagonal architecture**, established libraries over homegrown code | Maintainability; keep the codebase lean by depending on `cobra`, `viper`, `huh`, `age`, `go-keyring`, `fsnotify`, `afero`. |
| D7 | **Empty include = sync nothing** | Safety: removing the last include pattern must never silently sync work repos. `*` opts into everything. (Shipped in P0.) |

## Rejected alternatives

- **Auto-relocating the user's repos under `/dev`** to make paths line up — out of
  scope and dangerous; D1's translation layer solves the same problem without
  touching user code.
- **Injecting base prompts into session JSONL** to paper over path/home
  differences — fragile and pollutes history; D1 handles it via the path map.
- **Relying on git `merge=union`** as the long-term conflict strategy —
  incompatible with encryption (D2) and non-git providers (D4).

## Target architecture (hexagonal)

```
domain/                 Pure business rules, no I/O.
  session.go            Session, SessionID
  project.go            ProjectKey (canonical), PathMapping
  manifest.go           Manifest (devices + their project selections)
  merge.go              MergePolicy (JSONL union, last-writer-wins)

ports/                  Interfaces the core depends on (driven side).
  storage.go            Storage: Pull/Push opaque blobs + manifest
  crypto.go             Crypto: Seal/Open([]byte)
  keystore.go           KeyStore: Get/Set/Generate device & chain keys
  claudestore.go        ClaudeStore: enumerate/read/write ~/.claude projects
  trigger.go            Trigger: install/uninstall auto-sync hooks
  prompter.go           Prompter: interactive Q&A (tour) vs non-interactive

adapters/               Concrete implementations (driving + driven).
  storage/git           git CLI backend (+ gh auto-create of a private repo)
  storage/s3            S3-compatible backend
  storage/gdrive        Google Drive backend
  crypto/age            filippo.io/age
  keystore/keychain     zalando/go-keyring
  claudestore/fs        spf13/afero-backed filesystem
  trigger/hook          Claude Code SessionStart/Stop hooks
  trigger/launchd       periodic launchd job
  trigger/fsnotify      real-time watcher daemon
  prompter/huh          charmbracelet/huh forms (the welcome tour)
  prompter/noop         flag-driven, for hooks/launchd/CI

app/                    Use-case orchestration (the hexagon's inside edge).
  init.go               welcome tour, new-vs-join chain, key setup
  sync.go               pull → decrypt → merge → encrypt → push, with lock
  device.go             roster + per-device selection views

cmd/                    Cobra adapters → app services.
```

Dependency rule: `domain` imports nothing internal; `ports` import only `domain`;
`adapters` and `app` import `ports` + `domain`; `cmd` wires concrete adapters
into `app`. Everything crosses a port — no adapter calls another adapter.

## Selected dependencies

| Concern | Library |
|---|---|
| CLI | `spf13/cobra` (in use) |
| Config | `spf13/viper` (in use) |
| Interactive tour | `charmbracelet/huh` |
| Encryption | `filippo.io/age` |
| OS keychain | `zalando/go-keyring` |
| File watching | `fsnotify/fsnotify` |
| FS abstraction (tests) | `spf13/afero` |
| Release / Homebrew | `goreleaser/goreleaser` |

## Phase plan

Each phase ends with a green `/code-review` (high) and updated docs/tests.

### P0 — Review fixes ✅ (shipped, PR #1)
Empty-include safety, content-equality sync, glob validation, pull-before-mutate,
`.gitignore`, status platform.

### P1 — Hexagonal core + canonical keys + path translation ✅ (shipped)
Covers business reqs **#2, #3, #6, #7**.
- `internal/domain` (pure rules), `internal/ports` (interfaces),
  `internal/adapters` (claudefs, gitstore, gitident, nocrypto),
  `internal/app` (use cases). A `Crypto` passthrough port is already in place so
  P2 is a drop-in.
- Storage layout is `manifest` + `objects/<keyHash>/…` instead of a folder mirror.
- Canonical key = normalized git remote, else a home-relative path fallback. The
  true working dir is read from the session file's `cwd` (folder names are lossy).
- Per-device folder mapping in the manifest translates each project to this
  machine's folder on pull; a project is materialized only once it exists locally.
- `filter` accepts **directory paths**, not globs (#3). `device list` shows each
  device's include/exclude roots (#6).
- **Acceptance met:** verified two devices with the same repo at different paths
  (`~/dev/github/widgets` vs `~/github/widgets`) cross-sync into each device's own
  folder, with unit + end-to-end tests.

### P2 — Encryption + keychain ✅ (shipped)
Covers **#9** (+ metadata protection).
- `adapters/agecrypto` (age X25519 via the `Crypto` port) and
  `adapters/keychain` (go-keyring). Key model: one chain identity, kept in the OS
  keychain (`CCSYNC_IDENTITY` env override for headless/CI), never in the repo.
- Push encrypts each session object; pull decrypts. The manifest is encrypted too,
  so project paths don't leak. Change detection uses a plaintext hash + stored
  mtime inside the encrypted manifest (age ciphertext is non-deterministic) — this
  also fixes the old git-checkout-mtime issue.
- Key lifecycle: `init --new-chain` generates and prints the identity;
  `init --join [--key]` imports it; `key show` / `key id` for transfer.
- **Acceptance met:** verified end-to-end that the remote holds only `.age`
  ciphertext (no secrets, cwd, or paths) and the manifest is age-encrypted; unit
  tests cover seal/open, tampering, wrong-key, and no-plaintext-at-rest.
- Note: this is a breaking change from P1's plaintext layout — re-`init` chains.

### P3 — Storage strategy + providers + gh auto-create ✅ (shipped)
Covers **#10, #5**.
- A provider-agnostic `blobstore.BlobStore` (List/Get/Put/Exists, content-MD5
  versions) with a `Mirror` that satisfies `ports.Storage`, so any blob backend
  looks like the git working copy to the core.
- Backends, selected by config: `gitstore` (default), `s3store`
  (aws-sdk-go-v2; AWS config chain), `gdrivestore` (Drive API, least-privilege
  `drive.file`, flat files keyed by relpath, OAuth token cached).
- `init --create-repo` makes a private GitHub repo via `gh` (#5); git hygiene
  moved into `gitstore`.
- **Acceptance:** git backend verified end-to-end through the factory; the
  Mirror engine is unit-tested with an in-memory blob fake (two mirrors = two
  devices). S3/Drive adapters are SDK-thin and were **not** live-tested here (no
  cloud credentials in the dev environment) — they need a real bucket/folder to
  verify; the port + Mirror contract is the correctness anchor.

### P4 — Auto-sync triggers
Covers **#4** (config-driven per D5).
- `trigger/hook` (SessionStart/Stop), `trigger/launchd` (interval),
  `trigger/fsnotify` (debounced watcher); a sync lockfile serializes them.
- `ccsync auto enable/disable`, with selection + interval in config.
- **Acceptance:** enabling hooks makes a new/finished session sync with no manual
  command; concurrent triggers never corrupt state.

### P5 — Welcome tour + join/merge flows
Covers **#12** (+ #7 guidance).
- `prompter/huh` interactive `init`: new chain vs join; on join, choose
  **merge** vs **take-claude-base**; path-difference detection with auto-fix
  suggestions; key setup.
- `prompter/noop` keeps every prompt flag-addressable for automation.
- **Acceptance:** a fresh machine joins an existing encrypted chain through the
  tour alone; the same outcome is reproducible via flags only.

### P6 — Distribution + docs/memory-bank
Covers **#8, #13**.
- `goreleaser` + a Homebrew tap (`brew install melihemreguler/tap/ccsync`).
- Expand `docs/` into a memory-bank (architecture, decisions, per-phase notes)
  so future sessions/agents onboard from files, not chat history.
- **Acceptance:** `brew install` yields a working binary; a cold agent can
  implement a change using only the repo docs.

## Open questions (resolve before the owning phase)

- P2: passphrase-derived key vs generated keypair as the default? (Leaning
  keypair + Keychain, passphrase as a fallback for headless.)
- P3: which S3 SDK / Drive auth flow (service account vs OAuth device flow)?
- P4: default watcher debounce window and launchd interval.
