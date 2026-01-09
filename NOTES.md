# Development Notes

## Follow-up Items for Future PRs

### GlobalDataDir() Export Decision

**Context:** As part of the MCP OAuth token storage refactoring, we added a new exported function `config.GlobalDataDir()` to centralize the global data directory path logic (`~/.local/share/crush/` on Linux/macOS, `%LOCALAPPDATA%/crush/` on Windows).

**Current usage:**
- `GlobalConfigData()` in `internal/config/load.go`
- `cachePathFor()` in `internal/config/provider.go`
- `NewTokenStore()` in `internal/agent/tools/mcp/token_store.go`

**Question for maintainers:** How would you prefer to handle this?
1. Keep `GlobalDataDir()` as an exported function in the `config` package (current approach)
2. Make it unexported (`globalDataDir()`) and have callers use a different pattern
3. Move it to a different package (e.g., `internal/home` or a new `internal/paths` package)
4. Some other approach

---

### OAuth Browser Auto-Open Behavior

**Issue:** Currently, when an MCP server requires OAuth authorization, the browser opens automatically without user confirmation or notice. This happens as a side effect of connecting to the MCP server and can be unexpected/jarring for users.

**Current behavior:**
- `DefaultAuthFlowOptions()` sets `OpenBrowser: true`
- When `StartAuthFlow()` is called, the browser opens immediately
- Only feedback is a log message that may not be visible in the TUI

**Recommended fix:** Add user confirmation or at minimum a visible notice before opening the browser. Options:
1. Show a TUI dialog prompting user to confirm before opening browser
2. Change default to `OpenBrowser: false`, display the URL, let user manually open or press a key to auto-open
3. Add a config option `oauth.auto_open_browser` (default: false)

**Location:** `internal/oauth/mcp/flow.go` (`StartAuthFlow`), `internal/agent/tools/mcp/init.go` (auth function setup)

---

## RFC References for OAuth/PKCE Implementation

### PKCE (RFC 7636)
- **RFC 7636 Section 4.2** - Client creates code verifier and code challenge
- **RFC 7636 Section 7.2** - Security considerations: clients MUST use S256 (SHA256) transform
- https://datatracker.ietf.org/doc/html/rfc7636#section-4.2
- https://datatracker.ietf.org/doc/html/rfc7636#section-7.2

### OAuth 2.0 Authorization Server Metadata (RFC 8414)
- Server metadata MAY include `code_challenge_methods_supported` but it's optional
- If not present, client should still use PKCE with S256 (best practice)
- https://datatracker.ietf.org/doc/html/rfc8414

### OAuth 2.0 Dynamic Client Registration (RFC 7591)
- **Section 3** - `code_challenge_methods_supported` is optional in client metadata
- https://datatracker.ietf.org/doc/html/rfc7591#section-3

### Implementation Decision
Our implementation always uses PKCE with S256 regardless of server metadata, as mandated by RFC 7636 Section 7.2. The `code_challenge_methods_supported` field from server discovery is informational only - we do not conditionally enable/disable PKCE based on it.

---

## MCP OAuth Limitations

### Localhost-Only Redirect URI

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
