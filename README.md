# claude-code-sync (`ccsync`)

Selective, multi-device sync for [Claude Code](https://www.claude.com/product/claude-code) session history.

Claude Code keeps your conversation history **locally** in `~/.claude/projects/` and
does not sync it across machines (unlike the claude.ai web history). `ccsync` mirrors
those session files between your devices through a storage backend you control —
but only the projects **you choose**, and it works even when the same project lives
at different paths on different machines.

## Why this exists

There are several "sync everything in `~/.claude`" tools already. This one is built
around requirements the others don't cleanly cover:

1. **A device control panel.** Devices form an explicit chain you can list and
   remove from (`ccsync device list` / `device remove`), and the list shows which
   directories each device syncs.
2. **Path-selective sync.** You choose which projects sync **by directory**
   (`--include ~/dev/github`), not by cryptic patterns. Work repositories under
   another root stay on the machine.
3. **Path-independent identity.** The same git project is recognized across
   machines by its **git remote**, not its folder path. So `~/dev/github/app` on
   one Mac and `~/github/app` on another — even under different usernames — sync as
   the same logical project, each landing in that machine's own folder so
   `claude --resume` finds it.
4. **End-to-end encryption.** Sessions and the manifest are encrypted with
   [age](https://age-encryption.org) before they leave your machine. A leaked
   remote reveals nothing — not even project paths. The chain key lives in your OS
   keychain, never in the repo.

## How it works

Each project is stored under a **canonical key** (its normalized git remote, or a
home-relative path fallback). A synced, per-device **manifest** maps that key to the
folder name each machine uses, so sessions are translated to the right place on pull.

```
  device A                 ┌────────────────────────┐                device B
  ~/.claude/projects       │  storage backend (git)  │       ~/.claude/projects
   include: ~/dev/github   │  ├── manifest           │   include: ~/github
                           │  └── objects/<key>/...  │
   ~/dev/github/app  ──────┤                         ├──────▶  ~/github/app
        (git remote = github.com/you/app on both → same canonical key)
```

- `pull` integrates the backend, then **decrypts** each project's objects into
  **this device's** folder for that canonical key.
- `push` **encrypts** your selected local sessions into `objects/<key>/`, records
  this device and its folder mapping in the (encrypted) manifest, and publishes.
- Change detection uses a plaintext hash kept inside the encrypted manifest (age
  ciphertext is non-deterministic), with a stored modification time for newness.
- Sync is **additive**: it never deletes session files. Excluded projects never
  reach storage.

> ⚠️ **Still, keep the storage backend private.** Contents are encrypted, but a
> private backend is good defense in depth. Treat the chain key like a password.

## Install

```sh
brew install melihemreguler/tap/ccsync          # Homebrew (macOS)
# or
go install github.com/melihemreguler/claude-code-sync@latest   # binary: ccsync
# or build from source
git clone https://github.com/melihemreguler/claude-code-sync
cd claude-code-sync && make install
```

## Quick start

1. Create a **private** repo to hold the data, e.g. `claude-sessions` — or let
   ccsync create one for you with `--create-repo claude-sessions` (needs the
   `gh` CLI).
2. On your first machine, run `ccsync init`. In a terminal it launches an
   interactive **welcome tour** (backend, new-vs-join chain, directories,
   auto-sync). Prefer flags? Pass them (or `--no-input` to skip the tour):

   ```sh
   ccsync init --repo git@github.com:you/claude-sessions.git \
     --device macbook-personal \
     --new-chain \
     --include ~/dev/github
   ```

   This prints a chain identity (a secret) — back it up. Add more roots with
   repeated `--include`, and carve out exceptions with `--exclude ~/dev/github/work`.
   An empty include list syncs nothing.

3. On every other machine, **join** the chain with that identity:

   ```sh
   ccsync key show            # on the first machine — copies the identity
   ccsync init --repo git@github.com:you/claude-sessions.git \
     --device imac-home --join --include ~/github
   # paste the identity when prompted (or pass --key)
   ```

   Transfer the identity over a trusted channel (AirDrop, a password manager).
   When joining, the tour asks how to reconcile: **merge** (combine this machine's
   history with the chain) or **claude-base** (publish this machine's history
   without importing the chain's yet — `--claude-base`).

4. From then on:

   ```sh
   ccsync sync     # pull, then push
   ```

## Storage backends

The backend is pluggable (`--backend`). Whichever you pick, content is encrypted
before it leaves your machine.

- **git** (default) — a private git repo. `--repo <url>`, or `--create-repo <name>`
  to make a private GitHub repo via the `gh` CLI.
- **s3** — an S3 (or S3-compatible) bucket: `--backend s3 --s3-bucket <b>
  [--s3-prefix ccsync] [--s3-region <r>]`. Credentials come from the standard AWS
  config chain (env, shared config, IAM role).
- **gdrive** — a Google Drive folder: `--backend gdrive --gdrive-folder <id>
  --gdrive-credentials <oauth-client.json>`. First use runs a one-time browser
  consent; the token is cached locally. Uses the least-privilege `drive.file` scope.

All devices in a chain must use the same backend and target.

## Auto-sync

Sync hands-free with any combination of triggers (you choose):

```sh
ccsync auto enable --hooks                 # sync on Claude Code session start/end
ccsync auto enable --launchd --interval 15m # periodic background sync
ccsync auto enable --watch                 # real-time, on file change
ccsync auto status
ccsync auto disable                        # remove all of the above
ccsync auto disable --watch                # or just one trigger
```

- **hooks** add `SessionStart → pull` and `SessionEnd → push` to your Claude Code
  `settings.json` (other hooks are preserved). These run **synchronously**, so a
  slow backend adds a little latency to session start/end — prefer **launchd** or
  **watch** if you want zero-latency, fully background syncing.
- **launchd** installs a periodic `ccsync sync` LaunchAgent.
- **watch** installs a keep-alive agent running `ccsync watch` (real-time sync,
  batched over a short debounce window); you can also run `ccsync watch` in a
  terminal.

The hook/agent commands embed the absolute path of the `ccsync` binary at enable
time; if you move or reinstall it, run `ccsync auto enable …` again. Avoid
installing `ccsync` to a path containing spaces.

A local lock serializes overlapping triggers, so concurrent runs on one machine
skip rather than collide.

## Commands

| Command | What it does |
|---|---|
| `ccsync init --repo <url>` | Set up this device, clone storage, first sync |
| `ccsync sync` | Pull remote changes, then push local ones |
| `ccsync pull` / `push` | One direction only |
| `ccsync import --all` | Also materialize chain projects not present on this device |
| `ccsync status` | Config, which projects sync/skip (with cwd + key), device chain |
| `ccsync device list` | The chain, plus each device's include/exclude dirs |
| `ccsync device remove <name>` | Drop a device from the chain |
| `ccsync auto enable/disable/status` | Manage auto-sync triggers (hooks, launchd, watcher) |
| `ccsync watch` | Foreground real-time watcher (debounced sync) |
| `ccsync key show` | Print the chain identity (secret) to join another device |
| `ccsync key id` | Print the chain's public id (age recipient) |
| `ccsync filter list` | Show include/exclude directory roots |
| `ccsync filter add --include <dir>` / `--exclude <dir>` | Add a root |
| `ccsync filter remove --include/--exclude <dir>` | Remove a root |

## Configuration

Per-machine config lives at `~/.config/ccsync/config.json` (not synced; honors
`CCSYNC_*` env overrides):

```json
{
  "device": "macbook-personal",
  "repoUrl": "git@github.com:you/claude-sessions.git",
  "chainId": "age1… (public; the secret identity lives in the keychain)",
  "claudeDir": "/Users/you/.claude",
  "workDir": "/Users/you/.local/share/ccsync/repo",
  "include": ["~/dev/github"],
  "exclude": []
}
```

For headless/CI use, set `CCSYNC_IDENTITY` to the chain identity to bypass the
keychain.

## Caveats

- **An empty include list syncs nothing** — deliberate, so you never start syncing
  work repos by accident.
- **A project must be opened on a device before foreign history lands there.**
  ccsync only knows a device's folder for a project once that project has a local
  session; until then the data waits safely in storage. To pull it down anyway, run
  `ccsync import --all` — it writes those projects to disk under the originating
  device's folder name, though `claude --resume` will only list them from a
  matching working directory.
- **Projects without a git remote** fall back to a home-relative path key, which
  does not auto-translate across structurally different layouts (e.g.
  `~/dev/github` vs `~/github`). Git-backed projects always do.
- **Guard the chain key.** It lives in your OS keychain. If you lose it, encrypted
  history can't be recovered; if it leaks, the chain is exposed. `ccsync key show`
  reveals it — handle with care.
- **Concurrent edits to one session are merged, not lost.** Session `.jsonl`
  files are combined record-by-record (union, deduped by record `uuid`), so if a
  session is continued on two machines before they sync, both sides' appended
  records are preserved and the devices converge. Non-session files still use
  last-writer-wins by modification time. Running the *same live* session on two
  machines simultaneously is still best avoided.
- **Avoid simultaneous syncs on S3/Drive backends.** The git backend rebases on
  conflict, but blob backends update the manifest last-writer-wins; a metadata
  race self-heals on the next sync but is best avoided. A lock is planned.
- Not an official Anthropic product.

## Architecture & roadmap

`ccsync` follows a hexagonal layout (`internal/domain` rules, `internal/ports`
interfaces, `internal/adapters` implementations, `internal/app` use cases). New
here? Start with the [memory bank](docs/memory-bank/README.md) — onboarding
context for contributors and AI agents. See [docs/ROADMAP.md](docs/ROADMAP.md) for
the phase plan and locked design decisions.

## License

MIT — see [LICENSE](LICENSE).
