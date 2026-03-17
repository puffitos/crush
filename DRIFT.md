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

`GetModel` now falls back to a suffix match so that bedrock model IDs
prefixed with a cross-region inference profile (`eu.`, `us.`, `ap.`)
are still resolved when looked up by their original unprefixed ID
from catwalk.

| Commit | Description |
|--------|-------------|
| `f7980211` | fix(config): resolve bedrock model lookup when region prefix is set |
