// Package mcp provides OAuth 2.0 functionality for MCP server authentication.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/charmbracelet/crush/internal/oauth"
)

const (
	// DefaultRedirectURI is the default redirect URI using the default callback port.
	DefaultRedirectURI = "http://localhost:19876/callback"
)

// Config holds the OAuth configuration for an MCP server.
type Config struct {
	ClientID             string
	ClientSecret         string
	AuthURL              string
	TokenURL             string
	Scopes               []string
	RedirectURI          string
	RegistrationEndpoint string // For dynamic client registration (RFC 7591)
}

// SupportsDynamicRegistration returns true if dynamic client registration is available.
func (c *Config) SupportsDynamicRegistration() bool {
	return c.RegistrationEndpoint != ""
}

// Validate validates and normalizes the OAuth configuration.
// It sets defaults for missing fields and validates constraints.
// Returns an error if the configuration is invalid.
func (c *Config) Validate() error {
	// Set default redirect URI if not specified
	if c.RedirectURI == "" {
		c.RedirectURI = DefaultRedirectURI
	}

	// Validate redirect URI (localhost, http only)
	if err := validateRedirectURI(c.RedirectURI); err != nil {
		return err
	}

	// Validate URL fields parse correctly (if set)
	if c.AuthURL != "" {
		if _, err := url.Parse(c.AuthURL); err != nil {
			return fmt.Errorf("invalid auth_url: %w", err)
		}
	}

	if c.TokenURL != "" {
		if _, err := url.Parse(c.TokenURL); err != nil {
			return fmt.Errorf("invalid token_url: %w", err)
		}
	}

	if c.RegistrationEndpoint != "" {
		if _, err := url.Parse(c.RegistrationEndpoint); err != nil {
			return fmt.Errorf("invalid registration_endpoint: %w", err)
		}
	}

	// Must have either ClientID or RegistrationEndpoint for dynamic registration
	if c.ClientID == "" && c.RegistrationEndpoint == "" {
		return fmt.Errorf("either client_id or registration_endpoint must be set")
	}

	return nil
}

// validateRedirectURI checks that the URI is valid for
// the OAuth2 authentication flow for a localhost client.
func validateRedirectURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("invalid redirect_uri: %w", err)
	}

	if u.Scheme != "http" {
		return fmt.Errorf("redirect_uri must use http scheme, got %q", u.Scheme)
	}

	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return fmt.Errorf("redirect_uri must be localhost or 127.0.0.1, got %q", host)
	}

	return nil
}

// authorizeURL generates the OAuth authorization URL with PKCE challenge (RFC 7636).
func authorizeURL(cfg Config, state string, challenge string) (string, error) {
	u, err := url.Parse(cfg.AuthURL)
	if err != nil {
		return "", fmt.Errorf("invalid OAuth authorization URL: %w", err)
	}

	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", cfg.RedirectURI)
	q.Set("state", state)

	if len(cfg.Scopes) > 0 {
		q.Set("scope", strings.Join(cfg.Scopes, " "))
	}

	// PKCE is mandatory per RFC 7636
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")

	u.RawQuery = q.Encode()
	return u.String(), nil
}

// exchangeToken exchanges an authorization code for an access token.
func exchangeToken(ctx context.Context, cfg Config, code, verifier string) (*oauth.Token, error) {
	slog.Debug("Starting exchangeToken procedure")

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", cfg.RedirectURI)
	data.Set("client_id", cfg.ClientID)

	if cfg.ClientSecret != "" {
		data.Set("client_secret", cfg.ClientSecret)
	}

	// PKCE is mandatory per RFC 7636
	data.Set("code_verifier", verifier)

	return doTokenRequest(ctx, cfg.TokenURL, data)
}

// RefreshToken refreshes an expired access token using the refresh token.
func RefreshToken(ctx context.Context, cfg Config, refreshToken string) (*oauth.Token, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	data.Set("client_id", cfg.ClientID)

	if cfg.ClientSecret != "" {
		data.Set("client_secret", cfg.ClientSecret)
	}

	return doTokenRequest(ctx, cfg.TokenURL, data)
}

func doTokenRequest(ctx context.Context, tokenURL string, data url.Values) (*oauth.Token, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token request failed: status %d, body: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		TokenType    string `json:"token_type"`
		Scope        string `json:"scope"`
	}

	if err = json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	token := &oauth.Token{
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		ExpiresIn:    tokenResp.ExpiresIn,
	}
	token.SetExpiresAt()

	return token, nil
}
