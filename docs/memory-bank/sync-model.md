# Sync model — the hard parts

## Canonical project keys (cross-device path independence)

Claude Code stores each project under `~/.claude/projects/<folder>`, where folder
is the absolute working directory with `/` and `.` replaced by `-`. That encoding
is **lossy** (can't be reversed), so we never decode it; instead we read the true
`cwd` from inside the session `.jsonl` (it carries a `cwd` field).

From the cwd we derive a **CanonicalKey**:
- the project's **git remote**, normalized (`git@github.com:a/b.git`,
  `https://github.com/a/b` → `github.com/a/b`). Same repo on two machines → same
  key, regardless of path or username. This is what makes `~/dev/github/app` and
  `~/github/app` sync as one project.
- if there's no git remote: a home-relative path key (`path:~/notes/x`) — portable
  across usernames but not across different directory layouts.

A folder may hold sessions pulled from other devices (with foreign cwds). The
*local* cwd is the one whose encoding equals the folder name; that disambiguation
lives in `claudefs.ReadCwd`.

## The manifest

An encrypted `manifest` blob at the storage root holds:
- **devices**: name, platform, timestamps, and each device's include/exclude roots
  (surfaced by `device list`).
- **projects**: keyed by canonical key → `{ display, folders{device→localFolder},
  objects{relpath→{hash, mtime}} }`.

`folders` is the path-translation table: on pull, a project is materialized into
*this* device's folder. A device that has never opened a project locally gets `""`
and is skipped (we can't know its folder yet); the data waits in storage.

`objects` drives change detection: age ciphertext is non-deterministic, so we
compare the **plaintext sha256** and use the stored **mtime** for newness. Keeping
mtime here (not on disk) also sidesteps git not preserving modification times.

## Encryption

age (X25519). One chain identity (secret) decrypts what's encrypted to its
recipient (public). The identity lives in the OS keychain (or `CCSYNC_IDENTITY`
for headless); never in the repo/config. Config stores only the public `chainId`.
Session objects and the manifest are both encrypted, so a leaked remote reveals no
content and no project paths. Object directory names are an **HMAC** of the key
(keyed by the identity), so they can't be confirmed by hashing guessed repo names.

## Conflict & concurrency

- Per-file: content-equal → skip; else newer (by mtime) wins. Sequential use is
  safe; concurrent edits to one live session are discouraged.
- Same machine: a `flock` lock serializes overlapping triggers (skip, not queue).
- Cross-device: the manifest is **sharded per device** (`manifests/<device>.age`),
  so two devices syncing at once write different files and never collide — git
  rebases cleanly instead of hitting an unmergeable binary (encrypted) manifest.
  Objects are content-addressed (written once; others skip via hash), so blobs
  don't collide either. Reads merge all shards; a legacy single `manifest` is read
  as a baseline so pre-shard chains keep working.

## Storage backends

Selected by config. `git` (default) is a working clone. `s3`/`gdrive` go through
`blobstore.Mirror`, which keeps a local mirror dir in sync with a `BlobStore`
(List/Get/Put/Exists, content-MD5 versions) — so the core treats every backend
like the git working copy.

## Auto-sync

Opt-in triggers, any combination (config-driven): Claude Code **hooks**
(SessionStart→pull, SessionEnd→push), a periodic **launchd** agent, and a
real-time **watch** (fsnotify, debounced). All run through the same lock.
