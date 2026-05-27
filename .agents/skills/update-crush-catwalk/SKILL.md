---
name: update-crush-catwalk
description: Sync the puffitos/crush fork with upstream charmbracelet/crush, ignoring any local catwalk fork. Updates crush from upstream, preserves our fork-only changes, rebuilds via the Taskfile (which embeds the version via ldflags), and installs the binary. Use when the user wants to update crush from upstream, bump versions, or install a fresh build.
---

# Update crush fork from upstream

This skill keeps the `puffitos/crush` fork in sync with upstream while
preserving our fork-only changes.

## Repository

| Repo | Remote (fork) | Remote (upstream) |
|------|---------------|-------------------|
| crush | `fork` → `github.com/puffitos/crush` | `origin` → `github.com/charmbracelet/crush` |

## Locating the repo

Do **not** hardcode paths. Resolve the repository at runtime:

```bash
CRUSH_DIR=$(git rev-parse --show-toplevel)
echo "crush: $CRUSH_DIR"
git -C "$CRUSH_DIR" remote -v
```

Confirm `origin` points to `charmbracelet/crush` and `fork` points to
`puffitos/crush`.

## Overview

Our fork carries the following custom changes (tracked in `DRIFT.md`):

- Bedrock provider tweaks in `internal/config/`
- MCP OAuth 2.0 support in `internal/oauth/mcp/`
- WakaTime integration in `internal/integrations/wakatime/`
- Zellij notifications in `internal/integrations/zellij/`
- OAuth browser-fallback notice in `internal/ui/dialog/oauth_notice.go`

`catwalk` should now follow upstream directly. Do **not** update or depend on
any local `catwalk` fork, and do **not** add a `replace charm.land/catwalk =>`
line unless the user explicitly asks.

---

## Step 1 — Pull fork first

```bash
cd "$CRUSH_DIR"
git pull fork main
```

This keeps local work aligned with the fork before merging upstream.

## Step 2 — Fetch upstream

```bash
git fetch origin
```

## Step 3 — Merge upstream

```bash
git merge origin/main
```

Resolve any conflicts while preserving the fork-only changes listed above.

## Step 4 — Ensure catwalk uses upstream

Inspect `go.mod`. If a `replace charm.land/catwalk => ...` directive exists,
remove it, then run:

```bash
go mod tidy
```

Verify `go.mod` now depends on upstream `charm.land/catwalk` only.

## Step 5 — Build and test

```bash
go build ./...
go test ./...
```

## Step 6 — Push to fork

```bash
git push fork main
```

---

## Step 7 — Build and install crush

Use the Taskfile so the version is baked in via `-ldflags`.

```bash
cd "$CRUSH_DIR"
rm -rf .task crush
task build
./crush --version
mv crush "$(which crush)"
crush --version
```

---

## Troubleshooting

### Merge blocked by local changes

If the working tree contains local modifications, preserve them first (for
example by committing them on the fork branch) before merging upstream.
Do not discard user work.

### `go mod tidy` changes catwalk versions

That is expected now. We no longer pin a forked `catwalk`; upstream
`charm.land/catwalk` should win.

### Installed binary reports an older version

Rebuild with `task build` and replace the active binary. Plain `go build`
may leave a misleading pseudo-version in the binary metadata.
