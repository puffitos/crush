# MCP OAuth Implementation - Pull Request Summary

## Overview

This PR introduces OAuth 2.0 support for MCP (Model Context Protocol) servers, enabling secure authentication with OAuth-protected MCP endpoints. The implementation follows RFC standards for OAuth 2.0, PKCE, and Dynamic Client Registration.

---

## Key Features

1. **OAuth 2.0 Authorization Code Flow with PKCE** - Secure authentication for native/desktop apps
2. **Dynamic Client Registration (RFC 7591)** - Automatic client registration with OAuth servers
3. **OAuth Server Discovery (RFC 8414)** - Auto-discovery of OAuth endpoints from MCP server URLs
4. **Token Persistence** - Access tokens, refresh tokens, and client credentials stored globally in `~/.local/share/crush/mcp.json` for reuse across sessions
5. **Automatic Token Refresh** - Transparent token refresh on expiration or 401 responses

---

## New Modules & Files

### `internal/oauth/mcp/` - Core OAuth Implementation

| File | Purpose |
|------|---------|
| `oauth.go` | OAuth configuration, token exchange, and refresh logic |
| `flow.go` | Complete authorization flow orchestration (PKCE, browser, callback) |
| `discovery.go` | RFC 8414 OAuth server metadata discovery and validation |
| `register.go` | RFC 7591 Dynamic Client Registration |
| `callback.go` | Local HTTP server to receive OAuth callbacks |
| `browser.go` | Cross-platform browser opening utility |
| `pkce.go` | PKCE code verifier and challenge generation (RFC 7636) |

### `internal/agent/tools/mcp/` - MCP Integration

| File | Purpose |
|------|---------|
| `token_store.go` | Global token persistence for OAuth data |
| `oauth_transport.go` | HTTP RoundTripper that injects OAuth tokens into requests |

---

## How It Works

1. **MCP Initialization** (`init.go`)
   - For HTTP/SSE MCP servers, checks if OAuth is enabled
   - Attempts OAuth discovery from `/.well-known/oauth-authorization-server`
   - Creates `OAuthTokenProvider` and wraps HTTP transport with `oauthRoundTripper`

2. **Token Acquisition**
   - If no valid token exists, triggers authorization flow
   - Opens browser to authorization URL with PKCE challenge
   - Local callback server receives authorization code
   - Exchanges code for tokens using PKCE verifier

3. **Token Usage**
   - `oauthRoundTripper` injects `Authorization: Bearer <token>` header
   - On 401 or token expiration, automatically refreshes token
   - Tokens persisted to `mcp.json` for reuse across sessions

4. **Dynamic Client Registration**
   - If no `client_id` configured but server supports registration
   - Automatically registers as a public client
   - Stores received credentials for future use

---

## RFC Compliance

| RFC | Description | Implementation |
|-----|-------------|----------------|
| RFC 7636 | PKCE | Always enabled with S256 (mandatory) |
| RFC 8414 | OAuth Server Metadata | Discovery validation with issuer checks |
| RFC 7591 | Dynamic Client Registration | Auto-registration when no client_id |

---

## Files Changed

### New Files
- `internal/oauth/mcp/*.go` - OAuth implementation (8 files + tests)
- `internal/oauth/token.go` - Token struct
- `internal/agent/tools/mcp/token_store.go` - Token persistence
- `internal/agent/tools/mcp/oauth_transport.go` - HTTP transport wrapper

### Modified Files
- `internal/config/load.go` - Added `GlobalDataDir()` for centralized data path
- `internal/config/provider.go` - Refactored to use `GlobalDataDir()`
- `internal/config/config.go` - Added `MCPOAuthConfig` struct for MCP OAuth settings
- `internal/agent/tools/mcp/init.go` - OAuth integration during MCP initialization

---

## Configuration

MCP OAuth can be configured in `crush.json`. Here's the complete `MCPConfig` struct with all available fields:

```json
{
  "mcp": {
    "my-server": {
      "command": "npx",
      "args": ["-y", "@example/mcp-server"],
      "env": {
        "API_KEY": "your-api-key"
      },
      "type": "http",
      "url": "https://mcp.example.com",
      "disabled": false,
      "disabled_tools": ["tool-to-disable"],
      "timeout": 30,
      "headers": {
        "X-Custom-Header": "value"
      },
      "oauth": {
        "enabled": true,
        "client_id": "optional-if-dynamic-registration",
        "client_secret": "optional-for-confidential-clients",
        "authorization_url": "https://auth.example.com/authorize",
        "token_url": "https://auth.example.com/token",
        "scopes": ["openid", "profile"],
        "redirect_uri": "http://localhost:19876/callback"
      }
    }
  }
}
```

### MCPConfig Fields

| Field | Type | Description |
|-------|------|-------------|
| `command` | string | Command to execute for stdio MCP servers |
| `args` | string[] | Arguments to pass to the MCP server command |
| `env` | object | Environment variables to set for the MCP server |
| `type` | string | Type of MCP connection: `stdio`, `sse`, or `http` (required) |
| `url` | string | URL for HTTP or SSE MCP servers |
| `disabled` | bool | Whether this MCP server is disabled (default: false) |
| `disabled_tools` | string[] | List of tools from this MCP server to disable |
| `timeout` | int | Timeout in seconds for MCP server connections (default: 15) |
| `headers` | object | HTTP headers for HTTP/SSE MCP servers |
| `oauth` | object | OAuth 2.0 configuration (see below) |

### MCPOAuthConfig Fields

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable OAuth 2.0 authentication (default: true with auto-discovery) |
| `client_id` | string | OAuth 2.0 client identifier (optional if server supports dynamic registration) |
| `client_secret` | string | OAuth 2.0 client secret (optional for public clients using PKCE) |
| `authorization_url` | string | OAuth 2.0 authorization endpoint URL (auto-discovered if not set) |
| `token_url` | string | OAuth 2.0 token endpoint URL (auto-discovered if not set) |
| `scopes` | string[] | OAuth 2.0 scopes to request |
| `redirect_uri` | string | OAuth 2.0 redirect URI for callback (default: `http://localhost:19876/callback`) |

### Minimal Configuration (Auto-Discovery)

If the MCP server supports RFC 8414 OAuth discovery, no OAuth config is needed:

```json
{
  "mcp": {
    "my-server": {
      "type": "http",
      "url": "https://mcp.example.com"
    }
  }
}
```

OAuth will be auto-discovered from `/.well-known/oauth-authorization-server`.

---

## Limitations

1. **Localhost-only redirect URIs** - Only `http://localhost` or `http://127.0.0.1` supported
2. **No Device Code Flow** - Servers requiring non-local redirects not supported
3. **Browser auto-opens** - Currently opens without user confirmation (see NOTES.md)

---

## Testing

All new functionality has unit tests:
- `discovery_test.go` - Discovery response validation
- `flow_test.go` - Auth flow and redirect URI parsing
- `oauth_test.go` - Config validation and authorize URL generation
- `pkce_test.go` - PKCE generation
- `register_test.go` - Dynamic client registration
- `token_store_test.go` - Token persistence
- `oauth_transport_test.go` - Token provider and transport

---

## Notes for Reviewers

### GlobalDataDir() Export Decision

As part of the MCP OAuth token storage refactoring, we added a new exported function `config.GlobalDataDir()` to centralize the global data directory path logic (`~/.local/share/crush/` on Linux/macOS, `%LOCALAPPDATA%/crush/` on Windows).

**Current usage:**
- `GlobalConfigData()` in `internal/config/load.go`
- `cachePathFor()` in `internal/config/provider.go`
- `NewTokenStore()` in `internal/agent/tools/mcp/token_store.go`

**Question for maintainers:** How would you prefer to handle this?
1. Keep `GlobalDataDir()` as an exported function in the `config` package (current approach)
2. Make it unexported (`globalDataDir()`) and have callers use a different pattern
3. Move it to a different package (e.g., `internal/home` or a new `internal/paths` package)

### OAuth Browser Auto-Open Behavior

**Issue:** Currently, when an MCP server requires OAuth authorization, the browser opens automatically without user confirmation or notice. This happens as a side effect of connecting to the MCP server and can be unexpected/jarring for users.

**Recommended fix:** Add user confirmation or at minimum a visible notice before opening the browser. Options:
1. Show a TUI dialog prompting user to confirm before opening browser
2. Change default to `OpenBrowser: false`, display the URL, let user manually open or press a key to auto-open
3. Add a config option `oauth.auto_open_browser` (default: false)

### MCP OAuth Limitations - Localhost-Only Redirect URI

The MCP OAuth implementation only supports localhost redirect URIs (`http://localhost` or `http://127.0.0.1`). The callback server binds to `localhost:<port>` and listens for the OAuth callback.

**Supported configurations:**
- `http://localhost:8080/callback` - Uses port 8080 with `/callback` path
- `http://localhost:9000/oauth/cb` - Uses port 9000 with custom `/oauth/cb` path
- `http://127.0.0.1:3000/callback` - Uses 127.0.0.1 with port 3000
- No redirect_uri configured - Uses random port with `/callback` path

**Not supported (will return an error):**
- `https://...` - HTTPS is not supported (callback server is HTTP only)
- `http://example.com/...` - Remote hosts are not supported
- `http://[::1]:...` - IPv6 is not supported

If an MCP server requires a remote/external redirect URI (e.g., `https://example.com/oauth/callback`), this implementation will not work. Such servers would need to use Device Code Flow or provide a manual token entry mechanism instead.

### RFC References

- **RFC 7636 (PKCE)** - https://datatracker.ietf.org/doc/html/rfc7636
- **RFC 8414 (OAuth Server Metadata)** - https://datatracker.ietf.org/doc/html/rfc8414
- **RFC 7591 (Dynamic Client Registration)** - https://datatracker.ietf.org/doc/html/rfc7591
