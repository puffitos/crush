# Fork Drift from Upstream

This document tracks features and fixes in our fork
(`puffitos/crush`) that do **not** exist in the upstream repository
(`charmbracelet/crush`).

## MCP OAuth 2.0 Support

OAuth 2.0 authentication for MCP HTTP servers (Notion, Telecontext,
etc.) with dynamic client registration (RFC 7591), PKCE (RFC 7636),
authorization server discovery (RFC 8414), persistent token storage,
and automatic refresh.

Includes a TUI dialog that surfaces the OAuth URL (with clipboard
copy and SSH port-forwarding hints) when the browser cannot be
opened.

| Commit | Description |
|--------|-------------|
| `a7603135` | feat: add MCP OAuth 2.0 support |
| `b6f464aa` | feat: add MCP OAuth 2.0 support with dynamic client registration |
| `1613a132` | feat(mcp): notify user when OAuth browser open fails |
| `20e6060b` | feat(mcp): show OAuth dialog when browser cannot open |

## WakaTime Integration

Tracks AI coding activity by intercepting file operations (view,
edit, write, grep, glob) and sending heartbeats to `wakatime-cli`.
Includes throttling (2 min for reads, always on writes), project
detection, and nil-safe service design.

| Commit | Description |
|--------|-------------|
| `3ab3f588` | feat(integrations): add WakaTime time tracking integration |
| `c146db21` | feat(integrations): add WakaTime time tracking integration |
| `b0b12930` | fix: remove duplicate WakaTime field after rebase |

## Bedrock Region Prefix Fix

Uses catwalk's `PrefixModelIDs` method (added in our catwalk fork
`puffitos/catwalk`) to prepend the cross-region inference profile
prefix (`eu.`, `us.`, `ap.`) to all model IDs **and** default model
references (`DefaultLargeModelID`, `DefaultSmallModelID`) in one
call. This keeps lookups consistent for both large and small models.

Requires `go.mod` replace directive pointing to `puffitos/catwalk`.

| Commit | Description |
|--------|-------------|
| `f7980211` | fix(config): resolve bedrock model lookup when region prefix is set |
| `d90a5e95` | refactor(config): use catwalk PrefixModelIDs for bedrock region prefix |
