# Changelog

All notable changes to ccsync, newest first. Follows semantic versioning.

## [v0.4.0] — record-level session merge

- Session `.jsonl` files are now merged **record-by-record** on pull instead of
  whole-file last-writer-wins. The new `domain.MergeSessionJSONL` unions records,
  deduping by their `uuid` (content hash for uuid-less lines). It is deterministic,
  commutative and idempotent, so a session continued on two devices before they
  sync keeps both sides' appended records and the devices converge. Non-session
  files keep last-writer-wins by modification time.
- Tests are now hermetic: the sync lock dir is injectable (`SetLockDir`) so a
  background auto-sync on a contributor's machine no longer collides with the
  per-user `sync.lock` during `go test`.

## [v0.3.1] — device remove on blob backends

- `device remove` now works on **all** backends. A `Delete` method was added to the
  `Storage` port and `BlobStore` interface (git stages + pushes the removal; S3 uses
  `DeleteObject`; Drive uses `Files.Delete`; the Mirror removes the blob locally and
  remotely). Previously a removed device's manifest shard reappeared on the next sync
  on S3/Drive because the Mirror was additive.

## [v0.3.0] — init UX + safety

- **Granular `auto disable`**: `--hooks` / `--launchd` / `--watch` disable just those
  triggers (no flags still disables all), so the auto-sync set is fully adjustable
  later. (The init tour's trigger picker was already multi-select — toggle with Space.)
- **Tour fix**: joining an existing chain no longer offers "create a new repo"
  (contradictory — the chain's data repo already exists); it asks only for that URL.
- **Wrong-key guard on join**: init now fails fast with a clear message if the
  provided identity can't decrypt the chain, instead of erroring later mid-sync.
- **Public-repo guard**: init checks a GitHub data repo's visibility; a public repo
  is refused (or, interactively, asks to confirm). Override with `--allow-public`.

## [v0.2.1] — path-traversal hardening

- Pull validates each object's relative path (`fileutil.SafeJoin`) before writing,
  so a corrupt or hostile manifest can't place files outside the project folder.
- Added regression tests for the `claudefs` cwd folder-match logic and `SafeJoin`.

## [v0.2.0] — per-device manifest shards (concurrency fix)

Fixes concurrent syncs failing with a binary manifest merge conflict.

- The single encrypted `manifest` blob is replaced by per-device shards under
  `manifests/<device>.age`. Each device writes only its own shard, so two devices
  syncing at once touch different files — git rebases cleanly instead of hitting an
  unmergeable binary conflict.
- Reads merge all shards (devices unioned, project folder maps unioned, per-object
  metadata resolved by newest mtime). A legacy single `manifest` is still read as a
  baseline, so existing chains keep working during migration.
- `device remove` now deletes that device's shard.

## [v0.1.2] — `import --all`

- New `ccsync import --all`: materializes chain projects this device has never
  opened locally, under the originating device's folder name. An escape hatch for
  browsing history on a fresh machine without first checking out and opening each
  project. (`claude --resume` still keys by absolute path, so it only lists
  imported sessions from a matching directory.)

## [v0.1.1] — wrong-repo safeguards

- **Stale work dir guard** (`gitstore`): if the local clone's `origin` doesn't
  match the configured repo URL, ccsync errors with a clear "remove it to re-clone"
  message instead of silently pushing to the old remote.
- **Non-ccsync repo guard** (`app`): refuses to operate when the backend points at
  something that isn't a dedicated data repo (e.g. a project or the ccsync source).

## [v0.1.0] — first release

The initial build-up, by phase (see [docs/ROADMAP.md](docs/ROADMAP.md)):

- **P0** — Cobra + Viper CLI with an `internal/` package layout.
- **P1** — Hexagonal core; path-independent **canonical project keys** (git remote)
  with a per-device manifest, so the same repo syncs across machines at different
  paths/usernames. Directory-path filtering; per-device dirs in `device list`.
- **P2** — **End-to-end encryption** with age; chain identity in the OS keychain;
  the manifest is encrypted too; object names are keyed (HMAC).
- **P3** — **Pluggable storage backends**: git (default), S3, Google Drive, via a
  `BlobStore`/`Mirror` abstraction; `init --create-repo` via the `gh` CLI.
- **P4** — **Auto-sync**: Claude Code hooks, periodic launchd, and a real-time
  watcher; a per-machine lock serializes overlapping triggers.
- **P5** — Interactive **welcome tour** (`init`); join-time merge vs claude-base.
- **P6** — **Homebrew** distribution (GoReleaser cask + tap), CI/release workflows,
  and the `docs/memory-bank/` onboarding docs.
