package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/charmbracelet/crush/internal/oauth"
	mcpoauth "github.com/charmbracelet/crush/internal/oauth/mcp"
)

// TokenProvider is the interface for getting and refreshing OAuth tokens.
type TokenProvider interface {
	// EnsureToken returns a valid token, loading from cache, refreshing, or
	// triggering authorization as needed. The token is persisted to storage.
	EnsureToken(ctx context.Context) (*oauth.Token, error)
	// RefreshToken refreshes an expired token.
	RefreshToken(ctx context.Context) (*oauth.Token, error)
}

// oauthRoundTripper wraps an http.RoundTripper to add OAuth authentication.
type oauthRoundTripper struct {
	provider TokenProvider
	base     http.RoundTripper
	mu       sync.Mutex
}

// NewOAuthRoundTripper creates a new OAuth-aware RoundTripper.
func NewOAuthRoundTripper(provider TokenProvider, base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &oauthRoundTripper{
		provider: provider,
		base:     base,
	}
}

// RoundTrip implements http.RoundTripper to transparently add OAuth authentication
// to outgoing HTTP requests. It handles token lifecycle automatically: retrieving
// tokens, refreshing expired tokens, and retrying requests on 401 responses.
func (rt *oauthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	token, err := rt.provider.EnsureToken(req.Context())
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth token: %w", err)
	}

	// Check if token is expired and try to refresh
	if token.IsExpired() {
		slog.Debug("OAuth token expired, refreshing", "mcp", req.URL.Host)
		newToken, rErr := rt.provider.RefreshToken(req.Context())
		if rErr != nil {
			return nil, fmt.Errorf("failed to refresh OAuth token: %w", rErr)
		}
		token = newToken
	}

	// Clone the request to avoid modifying the original
	req2 := req.Clone(req.Context())
	req2.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token.AccessToken))

	resp, err := rt.base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}

	// If we get a 401, try to refresh the token and retry once
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		slog.Debug("Got 401, attempting token refresh", "mcp", req.URL.Host)
		newToken, rErr := rt.provider.RefreshToken(req.Context())
		if rErr != nil {
			return nil, fmt.Errorf("token refresh after 401 failed: %w", rErr)
		}

		req3 := req.Clone(req.Context())
		req3.Header.Set("Authorization", fmt.Sprintf("Bearer %s", newToken.AccessToken))
		return rt.base.RoundTrip(req3)
	}

	return resp, nil
}

// OAuthTokenProvider implements TokenProvider for MCP OAuth.
type OAuthTokenProvider struct {
	name     string
	config   mcpoauth.Config
	store    *TokenStore
	token    *oauth.Token
	mu       sync.RWMutex
	authFunc func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error)
}

// NewOAuthTokenProvider creates a new token provider for an MCP server.
// It validates the OAuth configuration and returns an error if invalid.
// The store is required for token persistence.
func NewOAuthTokenProvider(name string, cfg mcpoauth.Config, store *TokenStore) (*OAuthTokenProvider, error) {
	if store == nil {
		return nil, fmt.Errorf("token store is required for MCP %q", name)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid OAuth config for MCP %q: %w", name, err)
	}

	return &OAuthTokenProvider{
		name:   name,
		config: cfg,
		store:  store,
	}, nil
}

// SetAuthFunc sets the function to call when authorization is needed.
// This allows the TUI to inject its OAuth flow.
func (p *OAuthTokenProvider) SetAuthFunc(fn func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.authFunc = fn
}

// ensureClientRegistration ensures we have a registered client_id.
// If dynamic registration is supported and we don't have a client_id, it registers one.
func (p *OAuthTokenProvider) ensureClientRegistration(ctx context.Context) error {
	// If we already have a client_id, we're good
	if p.config.ClientID != "" {
		return nil
	}

	// Try to load stored client credentials from MCPOAuthData
	data, err := p.store.Load(p.name)
	if err != nil {
		return fmt.Errorf("failed to load OAuth data for MCP %q: %w", p.name, err)
	}
	if data != nil && data.ClientID != "" {
		p.config.ClientID = data.ClientID
		p.config.ClientSecret = data.ClientSecret
		slog.Debug("Loaded stored client credentials", "mcp", p.name, "client_id", data.ClientID)
		return nil
	}

	// Check if we can do dynamic registration
	if !p.config.SupportsDynamicRegistration() {
		return fmt.Errorf("no client_id configured and dynamic registration not supported for MCP %q", p.name)
	}

	// Perform dynamic registration
	slog.Info("Registering OAuth client dynamically", "mcp", p.name)

	creds, err := mcpoauth.RegisterClient(ctx, p.config)
	if err != nil {
		return fmt.Errorf("dynamic client registration failed: %w", err)
	}

	// Save credentials (merge with existing data if any)
	saveData := &MCPOAuthData{
		ClientID:     creds.ClientID,
		ClientSecret: creds.ClientSecret,
	}
	if data != nil {
		// Preserve existing token data
		saveData.AccessToken = data.AccessToken
		saveData.RefreshToken = data.RefreshToken
		saveData.ExpiresIn = data.ExpiresIn
		saveData.ExpiresAt = data.ExpiresAt
	}
	if err = p.store.Save(p.name, saveData); err != nil {
		slog.Warn("Failed to save client credentials", "mcp", p.name, "error", err)
	}

	// Update config
	p.config.ClientID = creds.ClientID
	p.config.ClientSecret = creds.ClientSecret
	slog.Info("OAuth client registered successfully", "mcp", p.name, "client_id", creds.ClientID)

	return nil
}

// EnsureToken returns a valid token, loading from cache, refreshing, or
// triggering authorization as needed. The token is persisted to storage.
func (p *OAuthTokenProvider) EnsureToken(ctx context.Context) (*oauth.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Return cached token if valid
	if p.token != nil && !p.token.IsExpired() {
		return p.token, nil
	}

	// Try to load from store
	if token, err := p.loadOrRefreshStoredToken(ctx); err == nil && token != nil {
		return token, nil
	}

	// No valid token available, need to authorize
	if p.authFunc == nil {
		return nil, fmt.Errorf("no valid token available and no auth function configured for MCP %q", p.name)
	}

	// Ensure we have a client_id before starting auth flow
	if err := p.ensureClientRegistration(ctx); err != nil {
		return nil, err
	}

	token, err := p.authFunc(ctx, p.config)
	if err != nil {
		return nil, fmt.Errorf("authorization failed: %w", err)
	}

	p.token = token
	if err = p.saveToken(token); err != nil {
		slog.Warn("Failed to save token", "mcp", p.name, "error", err)
	}
	return p.token, nil
}

// loadOrRefreshStoredToken attempts to load a valid token from storage,
// or refresh an expired token if a refresh token is available.
// Returns (nil, nil) if no usable token is found.
func (p *OAuthTokenProvider) loadOrRefreshStoredToken(ctx context.Context) (*oauth.Token, error) {
	data, err := p.store.Load(p.name)
	if err != nil || data == nil || data.AccessToken == "" {
		return nil, nil
	}

	stored := dataToToken(data)

	// Valid token in store
	if !stored.IsExpired() {
		p.token = stored
		return p.token, nil
	}

	// Expired but no refresh token
	if stored.RefreshToken == "" {
		return nil, nil
	}

	// Try to refresh
	if err = p.ensureClientRegistration(ctx); err != nil {
		slog.Debug("Failed to ensure client registration for refresh", "mcp", p.name, "error", err)
		return nil, nil
	}

	newToken, err := mcpoauth.RefreshToken(ctx, p.config, stored.RefreshToken)
	if err != nil {
		slog.Debug("Failed to refresh stored token", "mcp", p.name, "error", err)
		return nil, nil
	}

	p.token = newToken
	if err = p.saveToken(newToken); err != nil {
		slog.Warn("Failed to save refreshed token", "mcp", p.name, "error", err)
	}
	return p.token, nil
}

// RefreshToken refreshes the current token.
func (p *OAuthTokenProvider) RefreshToken(ctx context.Context) (*oauth.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Ensure we have client credentials
	if err := p.ensureClientRegistration(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure client registration: %w", err)
	}

	var refreshToken string
	if p.token != nil && p.token.RefreshToken != "" {
		refreshToken = p.token.RefreshToken
	} else {
		data, err := p.store.Load(p.name)
		if err == nil && data != nil && data.RefreshToken != "" {
			refreshToken = data.RefreshToken
		}
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("no refresh token available for MCP %q", p.name)
	}

	newToken, err := mcpoauth.RefreshToken(ctx, p.config, refreshToken)
	if err != nil {
		return nil, err
	}

	p.token = newToken
	_ = p.saveToken(newToken)

	return newToken, nil
}

// saveToken saves the token while preserving client credentials.
func (p *OAuthTokenProvider) saveToken(token *oauth.Token) error {
	// Load existing data to preserve client credentials
	data, _ := p.store.Load(p.name)
	if data == nil {
		data = &MCPOAuthData{}
	}

	// Update token fields
	data.AccessToken = token.AccessToken
	data.RefreshToken = token.RefreshToken
	data.ExpiresIn = token.ExpiresIn
	data.ExpiresAt = token.ExpiresAt

	return p.store.Save(p.name, data)
}

// dataToToken converts MCPOAuthData to oauth.Token.
func dataToToken(data *MCPOAuthData) *oauth.Token {
	return &oauth.Token{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresIn:    data.ExpiresIn,
		ExpiresAt:    data.ExpiresAt,
	}
}
