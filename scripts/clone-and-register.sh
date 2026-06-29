#!/usr/bin/env bash
#
# clone-and-register.sh
#
# Clone all your GitHub repos into a directory (default ~/github), open each once
# in Claude Code so its project folder is created, then run `ccsync sync` so the
# chain's history for those repos aligns into THIS machine's folders (and shows up
# in Claude Code recents).
#
# Why: ccsync identifies projects by their git remote, but it can only place a
# project's history into a folder once that project exists locally as a Claude
# project. This script creates those folders in bulk.
#
# Requires: gh (authenticated), claude. ccsync is optional (used at the end).
# Compatible with macOS's default bash 3.2 and zsh.
#
# Usage:
#   ./clone-and-register.sh [dest_dir] [owner]
#     dest_dir  where to clone (default: ~/github)
#     owner     GitHub owner (default: the authenticated gh user)
#
# Notes:
#   - Registration runs `claude -p` once per repo, which makes a tiny model call
#     (counts toward usage). Set REGISTER=0 to skip it (then open repos yourself).
#   - Skips repos already cloned and projects already registered (idempotent).

set -euo pipefail

DEST="${1:-$HOME/github}"
OWNER="${2:-}"
REGISTER="${REGISTER:-1}"
CLAUDE_PROMPT="${CLAUDE_PROMPT:-ccsync: register this project}"

command -v gh >/dev/null 2>&1 || { echo "error: gh not found"; exit 1; }

mkdir -p "$DEST"
[ -n "$OWNER" ] || OWNER="$(gh api user --jq .login)"
echo "Owner: $OWNER   Destination: $DEST   Register: $REGISTER"

# Encode a path the way Claude Code names project folders ('/' and '.' -> '-').
enc() { printf '%s' "$1" | sed 's/[/.]/-/g'; }

# Non-archived, non-fork repos you own. Adjust the flags to taste:
#   drop --source to include forks; add --visibility private to limit scope.
gh repo list "$OWNER" --source --no-archived --limit 1000 \
  --json nameWithOwner --jq '.[].nameWithOwner' |
while read -r repo; do
  [ -n "$repo" ] || continue
  name="${repo##*/}"
  dir="$DEST/$name"

  if [ -d "$dir/.git" ]; then
    echo "✓ $name (already cloned)"
  else
    echo "→ cloning $name"
    if ! gh repo clone "$repo" "$dir" -- -q; then
      echo "  ✗ clone failed, skipping"
      continue
    fi
  fi

  proj="$HOME/.claude/projects/$(enc "$dir")"
  if [ "$REGISTER" != "1" ]; then
    :
  elif [ -d "$proj" ]; then
    echo "  • already registered with Claude"
  elif command -v claude >/dev/null 2>&1; then
    echo "  • registering with claude"
    ( cd "$dir" && claude -p "$CLAUDE_PROMPT" >/dev/null 2>&1 ) ||
      echo "  ! claude registration failed (open it manually later)"
  else
    echo "  ! claude not found; skipping registration"
  fi
done

if command -v ccsync >/dev/null 2>&1; then
  echo "→ ccsync sync"
  ccsync sync || true
else
  echo "(ccsync not found — run 'ccsync sync' yourself once installed)"
fi

echo "Done. Open Claude Code — your repos should appear in recents."
