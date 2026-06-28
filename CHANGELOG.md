# Changelog

All notable changes to ccsync. This project follows semantic versioning once it
reaches its first tagged release.

## [Unreleased]

The pre-release build-up, by phase (see [docs/ROADMAP.md](docs/ROADMAP.md)):

- **P0** — Cobra + Viper CLI with an `internal/` package layout; initial review
  fixes (empty-include safety, content-equality sync, glob validation).
- **P1** — Hexagonal core; path-independent **canonical project keys** (git
  remote) with a per-device manifest, so the same repo syncs across machines at
  different paths/usernames. Filtering by **directory path**; per-device dirs in
  `device list`.
- **P2** — **End-to-end encryption** with age; chain identity in the OS keychain;
  the manifest is encrypted too; object names are keyed (HMAC).
- **P3** — **Pluggable storage backends**: git (default), S3, Google Drive, via a
  `BlobStore`/`Mirror` abstraction; `init --create-repo` via the `gh` CLI.
- **P4** — **Auto-sync**: Claude Code hooks, periodic launchd, and a real-time
  watcher; a per-machine lock serializes overlapping triggers.
- **P5** — Interactive **welcome tour** (`init`); join-time merge vs claude-base.
- **P6** — **Homebrew** distribution (GoReleaser cask + tap), CI/release
  workflows, and the `docs/memory-bank/` onboarding docs.
