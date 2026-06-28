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

### P1 — Hexagonal core + canonical keys + path translation
Covers business reqs **#2, #3, #6, #7**.
- Introduce `domain`, `ports`, `app`; move the existing engine behind ports.
- Repo layout becomes `manifest` + `objects/<projectKey>/<sessionId>` instead of a
  raw folder mirror.
- `ProjectKey`: derive a stable id per project (e.g. repo basename + content
  salt), independent of absolute path.
- `PathMapping` per device resolves `ProjectKey ↔ local ~/.claude folder name`,
  handling `/dev/github` vs `/github` and differing usernames/home dirs.
- `filter add/remove` accept **paths**, not globs; pattern logic is internal (#3).
- `device list` shows each device's included/excluded project selections (#6),
  read from the (decrypted) manifest.
- **Acceptance:** two devices with different project paths + usernames sync the
  same logical sessions and `claude --resume` finds them on both.

### P2 — Encryption + keychain
Covers **#9** (+ metadata protection).
- `crypto/age` adapter; `keystore/keychain` adapter.
- Sync core switches to decrypt → merge → encrypt; manifest encrypted.
- Key lifecycle: generate on new chain; import (paste/QR/AirDrop) on join.
- **Acceptance:** a leaked remote reveals no plaintext sessions or project paths;
  `go test` covers seal/open round-trips and a tampered-blob failure.

### P3 — Storage strategy + providers + gh auto-create
Covers **#10, #5**.
- `Storage` port with `git` (default), `s3`, `gdrive` adapters.
- `init` auto-creates a **private** repo via `gh` when present (#5); clean manual
  fallback and clear messaging when it is not.
- **Acceptance:** same chain works end-to-end over at least git + one non-git
  backend selected purely by config.

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
