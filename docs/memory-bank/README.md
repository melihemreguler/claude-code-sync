# ccsync Memory Bank

Onboarding context for contributors — human or AI. Read these before changing
code; they capture the *why*, which the source alone doesn't.

## Read in this order

1. [architecture.md](architecture.md) — the hexagonal layout: domain, ports,
   adapters, app, and how a sync flows through them.
2. [sync-model.md](sync-model.md) — the hard parts: canonical project keys, the
   manifest, encryption, conflict handling, storage backends, auto-sync.
3. [onboarding.md](onboarding.md) — build/test/release, conventions, and the
   end-to-end test recipe.
4. [../ROADMAP.md](../ROADMAP.md) — locked design decisions (D1–D7), rejected
   alternatives, and the phase history (P0–P6).

## What ccsync is

A CLI that syncs Claude Code session history (`~/.claude/projects`) across a
chain of devices — **selectively** (by directory), **end-to-end encrypted**, and
**path-independent** (the same git repo on two machines syncs even at different
paths). Claude Code keeps this history local-only; ccsync fills that gap.

## One-paragraph mental model

Each project is identified by a **canonical key** (its normalized git remote).
Per-session files are encrypted and stored under that key in a pluggable backend
(git/S3/Drive); an encrypted **manifest** records, per device, which local folder
maps to each key (so sessions land in the right place on each machine) plus each
object's content hash and modification time (for change detection and newness).
Sync = pull (decrypt remote → local) then push (encrypt local → remote), under a
per-machine lock, triggered manually or automatically (hooks/launchd/watcher).
