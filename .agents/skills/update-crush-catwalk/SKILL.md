---
name: update-crush-catwalk
description: Sync the puffitos/catwalk and puffitos/crush forks with their upstream charmbracelet repositories, preserving our custom AWS Bedrock inference-profile changes. Creates a new awsfix tag in catwalk, updates the go.mod replace directive in crush, rebuilds crush, and installs the binary. Use when the user wants to update crush or catwalk from upstream, bump versions, or install a fresh build.
---

# Update crush and catwalk forks from upstream

This skill keeps the `puffitos/catwalk` and `puffitos/crush` forks in sync with
upstream while preserving our custom AWS Bedrock inference-profile patches.

## Repositories

| Repo | Local path | Remote (fork) | Remote (upstream) |
|------|-----------|---------------|-------------------|
| catwalk | `/home/bbressi/dev/repos/catwalk` | `origin` → `github.com/puffitos/catwalk` | `upstream` → `github.com/charmbracelet/catwalk` |
| crush | `/home/bbressi/dev/repos/crush` | `fork` → `github.com/puffitos/crush` | `origin` → `github.com/charmbracelet/crush` |

## Overview

Our forks carry the following custom changes (tracked in `DRIFT.md` for crush):

**catwalk** (`internal/providers/providers.go`, `internal/providers/configs/bedrock.json`):
- `bedrockProvider()` reads `AWS_REGION` / `AWS_DEFAULT_REGION` and prefixes
  model IDs with the correct cross-region inference profile prefix
  (`us.`, `eu.`, `jp.`, `au.`, `global.`).
- `bedrock.json` models carry a `"regions"` array listing which prefixes they
  support.

**crush** (`internal/config/`, `go.mod`):
- `go.mod` replaces `charm.land/catwalk` with `github.com/puffitos/catwalk@<tag>`.
- Bedrock model lookup adapted to prefixed IDs.
- MCP OAuth 2.0 support, WakaTime integration, OAuth browser-fallback notice.

---

## Step 1 — Update catwalk fork

### 1.1 Fetch upstream

```bash
cd /home/bbressi/dev/repos/catwalk
git fetch upstream
```

### 1.2 Identify custom changes

Save a patch of our changes relative to the common ancestor:

```bash
MERGE_BASE=$(git merge-base upstream/main origin/main)
git diff "$MERGE_BASE"..origin/main \
  -- internal/providers/configs/bedrock.json \
     internal/providers/providers.go \
     pkg/catwalk/provider.go \
  > /tmp/catwalk-custom.patch
```

### 1.3 Reset to upstream

```bash
git reset --hard upstream/main
```

### 1.4 Apply custom patch

```bash
git apply --3way /tmp/catwalk-custom.patch
```

Resolve any conflicts manually. Key invariants to preserve:
- `bedrockProvider()` in `providers.go` must read `AWS_REGION`/`AWS_DEFAULT_REGION`
  and call `bedrockRegionPrefix()` to prefix model IDs.
- `bedrock.json` models must carry `"regions": [...]` arrays.
- If `pkg/catwalk/provider.go` needs a `Regions []string` field on `Model`,
  add it — but note that `providers.go` uses an inline struct to parse JSON
  and does **not** require the field on `catwalk.Model`.

### 1.5 Build and test

```bash
go build ./...
go test ./...
```

### 1.6 Commit and tag

Determine the new tag. Pattern is `v<upstream-version>-awsfix`. Check the
latest upstream tag:

```bash
git describe --tags upstream/main --abbrev=0
# e.g. v0.34.3  →  new tag: v0.34.3-awsfix
```

```bash
git add internal/providers/configs/bedrock.json \
        internal/providers/providers.go \
        pkg/catwalk/provider.go   # if changed
git commit -m "chore: sync with upstream <version>, preserve bedrock inference profile fixes"
git tag v<version>-awsfix
```

### 1.7 Push to fork

```bash
git push --force-with-lease origin main
git push origin v<version>-awsfix
```

---

## Step 2 — Update crush fork

### 2.1 Fetch upstream

```bash
cd /home/bbressi/dev/repos/crush
git fetch origin   # origin = charmbracelet/crush (upstream)
```

### 2.2 Merge upstream

```bash
git merge origin/main
```

Resolve any conflicts. Our custom files are in:
- `internal/config/provider.go` — bedrock region prefix logic
- `internal/config/load.go` — catwalk `ApplyBedrockRegion` call
- `internal/oauth/mcp/` — MCP OAuth 2.0
- `internal/integrations/wakatime/` — WakaTime
- `internal/ui/dialog/oauth_notice.go` — OAuth browser fallback dialog
- `go.mod` — `replace charm.land/catwalk => github.com/puffitos/catwalk <tag>`

### 2.3 Bump catwalk dependency

Edit `go.mod` — update the `replace` directive to the new awsfix tag:

```
replace charm.land/catwalk => github.com/puffitos/catwalk v<version>-awsfix
```

Then run:

```bash
go mod tidy
```

Verify `go.sum` now contains the new `github.com/puffitos/catwalk` entry.

### 2.4 Build and test

```bash
go build ./...
go test ./...
```

### 2.5 Commit

```bash
git add go.mod go.sum
git commit -m "chore(deps): bump catwalk to v<version>-awsfix"
```

### 2.6 Push to fork

```bash
git push fork main
```

---

## Step 3 — Build and install crush

```bash
cd /home/bbressi/dev/repos/crush
CGO_ENABLED=0 GOEXPERIMENT=greenteagc go build -o "$(which crush)" .
crush --version   # verify
```

---

## Troubleshooting

### `go apply` patch fails with "does not apply"

Use `--3way` flag:
```bash
git apply --3way /tmp/catwalk-custom.patch
```

If conflicts remain, open the conflicted files and ensure:
1. `bedrockProvider()` body is our custom version (not just `return loadProviderFromConfig(bedrockConfig)`).
2. `bedrock.json` models have `"regions": [...]`.

### `go mod tidy` fails to download new catwalk tag

The tag must be pushed to `github.com/puffitos/catwalk` **before** running
`go mod tidy`. Check with:
```bash
git -C /home/bbressi/dev/repos/catwalk tag --list | grep awsfix
```

### Build error: `assignment mismatch` in `providers.go`

This means `catwalk.Model.Regions` type mismatch. Our `providers.go` uses
`[]string` for regions (parsed from an inline struct), not `map[string]string`.
Ensure `pkg/catwalk/provider.go` — if it has a `Regions` field — declares it
as `[]string`, not `map[string]string`.

### Rebase vs. merge strategy

Do **not** use `git rebase` on the catwalk fork — our branch has many
intermediate commits that conflict with upstream's equivalent commits
(they look identical but have diverged hashes). The correct strategy is:

1. Save the diff from the common ancestor.
2. Hard-reset to `upstream/main`.
3. Apply the saved diff with `--3way`.
