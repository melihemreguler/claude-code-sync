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

- `pull` integrates the backend, then copies each project's objects into **this
  device's** folder for that canonical key.
- `push` copies your selected local sessions into `objects/<key>/`, records this
  device and its folder mapping in the manifest, and publishes.
- Sync is **additive**: it never deletes session files. Excluded projects never
  reach storage.

> ⚠️ **Keep the storage backend private.** It contains your conversation history —
> code, command output, and (until encryption lands in a later phase) project
> paths. Don't add machines with confidential code unless policy allows it.

## Install

```sh
go install github.com/melihemreguler/claude-code-sync@latest   # binary: ccsync
```

Or build from source:

```sh
git clone https://github.com/melihemreguler/claude-code-sync
cd claude-code-sync && make install
```

## Quick start

1. Create a **private** repo to hold the data, e.g. `claude-sessions`.
2. On your first machine:

   ```sh
   ccsync init --repo git@github.com:you/claude-sessions.git \
     --device macbook-personal \
     --include ~/dev/github
   ```

   Add more roots with repeated `--include`, and carve out exceptions with
   `--exclude ~/dev/github/work`. An empty include list syncs nothing.

3. On every other machine, run `init` with a different `--device` name and that
   machine's own include path(s).

4. From then on:

   ```sh
   ccsync sync     # pull, then push
   ```

## Commands

| Command | What it does |
|---|---|
| `ccsync init --repo <url>` | Set up this device, clone storage, first sync |
| `ccsync sync` | Pull remote changes, then push local ones |
| `ccsync pull` / `push` | One direction only |
| `ccsync status` | Config, which projects sync/skip (with cwd + key), device chain |
| `ccsync device list` | The chain, plus each device's include/exclude dirs |
| `ccsync device remove <name>` | Drop a device from the chain |
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
  "claudeDir": "/Users/you/.claude",
  "workDir": "/Users/you/.local/share/ccsync/repo",
  "include": ["~/dev/github"],
  "exclude": []
}
```

## Caveats

- **An empty include list syncs nothing** — deliberate, so you never start syncing
  work repos by accident.
- **A project must be opened on a device before foreign history lands there.**
  ccsync only knows a device's folder for a project once that project has a local
  session; until then the data waits safely in storage.
- **Projects without a git remote** fall back to a home-relative path key, which
  does not auto-translate across structurally different layouts (e.g.
  `~/dev/github` vs `~/github`). Git-backed projects always do.
- **Don't run the same session on two machines at once.** Sync compares content
  (and mtime as a tiebreaker); concurrent edits to one live session are best
  avoided. Encryption + a content-addressed engine are on the roadmap.
- Not an official Anthropic product.

## Architecture & roadmap

`ccsync` follows a hexagonal layout (`internal/domain` rules, `internal/ports`
interfaces, `internal/adapters` implementations, `internal/app` use cases). See
[docs/ROADMAP.md](docs/ROADMAP.md) for the phase plan and design decisions
(encryption, storage providers, auto-sync, the welcome tour, and more).

## License

MIT — see [LICENSE](LICENSE).
