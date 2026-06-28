# claude-code-sync (`ccsync`)

Selective, multi-device sync for [Claude Code](https://www.claude.com/product/claude-code) session history.

Claude Code keeps your conversation history **locally** in `~/.claude/projects/` and
does not sync it across machines (unlike the claude.ai web history). `ccsync` mirrors
those session files between your devices through a git repository you control — but
only the projects **you choose**.

## Why this exists

There are several "sync everything in `~/.claude`" tools already. This one is built
around two requirements the others don't cleanly cover:

1. **A device control panel.** Devices form an explicit chain you can list and
   remove from (`ccsync device list` / `device remove`). The chain lives in the
   synced repo so every machine sees the same roster.
2. **Path-selective sync.** You sync sessions under *specific* project paths and
   leave the rest alone. Claude stores each project under a folder whose name
   embeds the full path (e.g. `-Users-me-dev-github-foo`), so a glob like
   `*github*` syncs everything under `~/dev/github` while your work repos
   (e.g. `*turknet*`) never leave the machine.

Cloud transport is just a git remote (a **private** GitHub repo), so there's no
extra service to run and the data sits where you already trust your code.

## How it works

```
  device A  ──push──▶  ┌──────────────────────┐  ◀──pull──  device B
  ~/.claude/projects   │  private git repo     │   ~/.claude/projects
                       │  ├── devices.json     │
   filter: *github*    │  └── projects/        │   filter: *github*
                       │      └── <matching>/  │
                       └──────────────────────┘
```

- `pull` rebases the repo, then copies any **newer** session files into `~/.claude`.
- `push` copies your matching local sessions into the repo, records this device in
  `devices.json`, commits, and pushes (auto-rebasing once if the remote moved).
- Session logs (`*.jsonl`) are configured for git **union-merge**, so concurrent
  appends from two devices merge instead of conflicting.
- Sync is **additive**: `ccsync` never deletes session files. Excluded projects are
  never copied into the repo at all.

> ⚠️ **Path matching is your responsibility.** Sessions are keyed by absolute path.
> If two machines use the same username and clone repos to the same location, the
> folder names match and `claude --resume` finds the synced sessions. If your paths
> differ across machines, the sessions still sync but appear under different project
> folders.
>
> ⚠️ **Keep the data repo private.** It contains your full conversation history,
> including code and command output. Do not add work machines with confidential
> code unless your employer's policy allows it.

## Install

```sh
go install github.com/melihemreguler/claude-code-sync@latest
# the binary is named `ccsync`
```

Or build from source:

```sh
git clone https://github.com/melihemreguler/claude-code-sync
cd claude-code-sync
make install   # builds ./ccsync and copies it to $GOPATH/bin
```

## Quick start

1. Create a **private** repo on GitHub to hold the data, e.g. `claude-sessions`.
2. On your first machine:

   ```sh
   ccsync init --repo git@github.com:you/claude-sessions.git --device macbook-personal
   ```

   `--include` defaults to `*github*`. Override it if your projects live elsewhere:

   ```sh
   ccsync init --repo git@github.com:you/claude-sessions.git \
     --device macbook-personal \
     --include '*github*,*personal*' \
     --exclude '*turknet*,*work*'
   ```

3. On every other machine, run the same `init` with a different `--device` name.
   The first sync pulls everything already in the repo.

4. From then on, just run:

   ```sh
   ccsync sync
   ```

   (Pull, then push.) Wire it into a shell alias, a `cron`/`launchd` job, or run it
   by hand before and after a session.

## Commands

| Command | What it does |
|---|---|
| `ccsync init --repo <url>` | Set up this device, clone the data repo, first sync |
| `ccsync sync` | Pull remote changes, then push local ones |
| `ccsync pull` | Apply remote sessions into `~/.claude` |
| `ccsync push` | Send local sessions to the remote |
| `ccsync status` | Show config, which projects sync/skip, and the device chain |
| `ccsync device list` | Show the device chain (the control panel) |
| `ccsync device remove <name>` | Drop a device from the chain |
| `ccsync filter list` | Show include/exclude patterns |
| `ccsync filter add --include <glob>` | Add an include pattern |
| `ccsync filter add --exclude <glob>` | Add an exclude pattern |
| `ccsync filter remove --include/--exclude <glob>` | Remove a pattern |

Filters use shell-style globs (`path.Match`) against project **folder names**.
Run `ccsync status` to preview exactly what will and won't sync.

## Configuration

Per-machine config lives at `~/.config/ccsync/config.json` and is **not** synced:

```json
{
  "device": "macbook-personal",
  "repoUrl": "git@github.com:you/claude-sessions.git",
  "claudeDir": "/Users/you/.claude",
  "workDir": "/Users/you/.local/share/ccsync/repo",
  "include": ["*github*"],
  "exclude": []
}
```

## Caveats

- **An empty include list syncs nothing.** This is deliberate: removing your last
  include pattern should never silently start syncing every project. Use an
  explicit `*` pattern to include everything.
- **Don't run the same session on two machines at once.** Sync compares file
  content (and, as a tiebreaker, modification time) to decide what is newer.
  Because git does not preserve modification times, this heuristic is reliable
  for sequential use but can mis-order genuinely concurrent edits to the *same*
  live session. Union-merge of `*.jsonl` is the backstop. Sync between sessions,
  not during. (A content-addressed sync engine that removes this caveat is
  planned — see the roadmap.)
- `ccsync` syncs files; it does not migrate paths. See the path note above.
- This is not an official Anthropic product.

## License

MIT — see [LICENSE](LICENSE).
