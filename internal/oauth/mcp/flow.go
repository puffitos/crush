package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"time"

	"github.com/charmbracelet/crush/internal/oauth"
)

const (
	// DefaultAuthTimeout is the default timeout for the OAuth authorization flow.
	DefaultAuthTimeout = 5 * time.Minute
)

// AuthFlowOptions configures the OAuth authorization flow.
type AuthFlowOptions struct {
	// Timeout for the entire auth flow (use DefaultAuthTimeout as a default)
	Timeout time.Duration
	// OpenBrowser controls whether to automatically open the browser
	OpenBrowser bool
	// OnAuthURL is called with the authorization URL (for displaying to user)
	OnAuthURL func(url string)
}

// DefaultAuthFlowOptions returns the default options for the auth flow.
func DefaultAuthFlowOptions() AuthFlowOptions {
	return AuthFlowOptions{
		Timeout:     DefaultAuthTimeout,
		OpenBrowser: true,
	}
}

// StartAuthFlow initiates the complete OAuth authorization flow.
// It starts a local callback server, opens the browser for authorization,
// waits for the callback, and exchanges the code for tokens.
func StartAuthFlow(ctx context.Context, cfg Config, opts AuthFlowOptions) (*oauth.Token, error) {
	// Create a context with timeout for the entire flow
	flowCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	// Generate PKCE verifier and challenge (mandatory per RFC 7636)
	verifier, challenge := generatePKCE()

	// Generate random state for CSRF protection
	state := generateState()

	// Parse redirect URI to extract port and path (already validated by Config.Validate())
	callbackPort, callbackPath := parseRedirectURI(cfg.RedirectURI)

	// Start the callback server
	server, err := newCallbackServer(flowCtx, callbackPort, callbackPath)
	if err != nil {
		return nil, fmt.Errorf("failed to start callback server: %w", err)
	}
	defer server.Close()
	server.Start()

	// Use the server's redirect URI (includes actual port if we used random)
	cfg.RedirectURI = server.RedirectURI()

	// Generate authorization URL
	authURL, err := authorizeURL(cfg, state, challenge)
	if err != nil {
		return nil, fmt.Errorf("failed to generate authorization URL: %w", err)
	}

	slog.Info("OAuth authorization required",
		"auth_url", authURL,
		"redirect_uri", cfg.RedirectURI,
	)

	// Notify caller of the auth URL
	if opts.OnAuthURL != nil {
		opts.OnAuthURL(authURL)
	}

	// Open browser if requested
	if opts.OpenBrowser {
		if err = openBrowser(authURL); err != nil {
			slog.Warn("Failed to open browser automatically", "error", err)
			// Continue anyway - user can copy the URL
		}
	}

	// Wait for the callback
	result, err := server.waitForCallback(flowCtx)
	if err != nil {
		return nil, fmt.Errorf("failed waiting for OAuth callback: %w", err)
	}

	// Check for errors in the callback
	if result.Error != "" {
		return nil, fmt.Errorf("failed OAuth authorization: %s", result.Error)
	}

	// Verify state to prevent CSRF
	if result.State != state {
		return nil, fmt.Errorf("mismatch in OAuth state")
	}

	// Exchange the code for tokens
	token, err := exchangeToken(flowCtx, cfg, result.Code, verifier)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange authorization code: %w", err)
	}

	slog.Info("OAuth authorization successful")

	return token, nil
}

// parseRedirectURI parses a validated redirect URI into port and path components.
// The URI must be validated via Config.Validate() before calling this function.
func parseRedirectURI(redirectURI string) (port int, path string) {
	u, _ := url.Parse(redirectURI) // Already validated by Config.Validate()

	// Extract port (0 if not specified, means use random)
	if p := u.Port(); p != "" {
		port, _ = strconv.Atoi(p) // Already validated
	}

	// Extract path (default to /callback if empty)
	path = u.Path
	if path == "" {
		path = "/callback"
	}

	return port, path
}

// generateState generates a random state string for CSRF protection.
// It panics if the system's random number generator fails, which indicates
// a fundamental system problem that cannot be recovered from.
func generateState() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
