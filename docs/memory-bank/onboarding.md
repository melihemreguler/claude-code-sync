# Onboarding — build, test, release

## Prerequisites

- Go (see `go.mod` for the version). macOS for the keychain/launchd adapters;
  the core builds and tests on Linux too (CI runs on Ubuntu).

## Everyday commands

```sh
make build      # build ./ccsync
make test       # go test ./...
make vet        # go vet ./...
make install    # go install
go test ./...   # all unit tests
```

## Conventions

- Hexagonal boundaries (see architecture.md). Keep `internal/domain` pure.
- New behavior belongs behind a port if it touches I/O or has multiple impls.
- Tests prefer fakes via `app.NewWith` / in-memory adapters over hitting the
  network or the real keychain.
- `gofmt` everything; commits are conventional-ish; each phase lands via a PR
  reviewed with `/code-review` until clean.

## End-to-end test recipe (no cloud, no real keychain)

Use a bare git repo as the backend and an env-provided identity so nothing touches
the real keychain:

```sh
export CCSYNC_IDENTITY="$(go run filippo.io/age/cmd/age-keygen@v1.3.1 | grep AGE-SECRET-KEY)"
# point HOME at a sandbox, init two devices against one bare data.git with
# projects at different paths, then assert sessions cross-sync into each device's
# own folder. (See the project's PR history for full scripts.)
```

`CCSYNC_IDENTITY` bypasses the keychain; non-TTY stdin or `--no-input` skips the
welcome tour, so `init` runs purely from flags in scripts.

## Verifying the S3 / Google Drive backends (live)

The blob backends are SDK-thin and can't be exercised without real credentials, so
they ship a shared **contract test** (`blobstoretest.Run`, covering
Put/Get/List/Exists/Delete) that runs against the in-memory reference on every CI
build, plus **gated integration tests** that run the same contract against a real
bucket/folder. They `t.Skip` unless their env vars are set:

```sh
# S3 (credentials from the standard AWS chain)
CCSYNC_S3_TEST_BUCKET=my-test-bucket CCSYNC_S3_TEST_REGION=us-east-1 \
  go test ./internal/adapters/s3store/ -run TestS3Contract -v

# Google Drive (first run opens a browser consent flow; token is cached)
CCSYNC_GDRIVE_TEST_FOLDER=<folderId> \
CCSYNC_GDRIVE_TEST_CREDENTIALS=client_secret.json \
  go test ./internal/adapters/gdrivestore/ -run TestGDriveContract -v
```

Use a throwaway bucket/folder — the tests write and delete `objects/contract/…`.
For a full end-to-end check, also run `ccsync init --backend s3|gdrive …` against a
sandbox and confirm a `sync` round-trips between two devices.

## Releasing (Homebrew)

1. Create the tap repo `melihemreguler/homebrew-tap` (once).
2. Add a repo secret `HOMEBREW_TAP_GITHUB_TOKEN` — a PAT with `repo` scope on the
   tap.
3. Tag and push: `git tag vX.Y.Z && git push origin vX.Y.Z`.
4. `.github/workflows/release.yml` runs GoReleaser (`.goreleaser.yaml`): builds
   darwin/linux × amd64/arm64, publishes a GitHub release, and pushes a Homebrew
   **cask** to the tap.
5. Validate config changes locally with
   `go run github.com/goreleaser/goreleaser/v2@latest check`.

Users then install with `brew install melihemreguler/tap/ccsync`, or
`go install github.com/melihemreguler/claude-code-sync@latest`.
