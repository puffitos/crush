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
)

// discoveryResponse represents the OAuth 2.0 Authorization Server Metadata (RFC 8414).
// This is used internally for JSON unmarshaling during discovery.
type discoveryResponse struct {
	Issuer                 string   `json:"issuer"`
	AuthorizationEndpoint  string   `json:"authorization_endpoint"`
	TokenEndpoint          string   `json:"token_endpoint"`
	RegistrationEndpoint   string   `json:"registration_endpoint,omitempty"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported []string `json:"response_types_supported"`
}

// validateDiscoveryResponse validates the OAuth discovery response per RFC 8414.
// It checks all required fields and verifies the issuer matches the expected host.
// The scheme and host parameters are used to verify the issuer to prevent impersonation attacks.
func validateDiscoveryResponse(resp *discoveryResponse, scheme, host string) error {
	// Validate required fields per RFC 8414
	if resp.Issuer == "" {
		return fmt.Errorf("missing required issuer field")
	}
	if resp.AuthorizationEndpoint == "" {
		return fmt.Errorf("missing required authorization_endpoint field")
	}
	if resp.TokenEndpoint == "" {
		return fmt.Errorf("missing required token_endpoint field")
	}
	if len(resp.ResponseTypesSupported) == 0 {
		return fmt.Errorf("missing required response_types_supported field")
	}

	// Per RFC 8414 §3.3: The issuer value returned MUST be identical to the
	// authorization server's issuer identifier value into which the well-known
	// URI string was inserted. Since we discover from the root, we validate that
	// the issuer's scheme and host match to prevent impersonation attacks while
	// allowing legitimate issuers with path components (e.g., multi-tenant setups).
	expectedPrefix := fmt.Sprintf("%s://%s", scheme, host)
	if !strings.HasPrefix(resp.Issuer, expectedPrefix) {
		return fmt.Errorf("issuer %q does not match expected host %q", resp.Issuer, expectedPrefix)
	}

	return nil
}

// DiscoverOAuth attempts to discover OAuth configuration from the server's well-known endpoint.
// It returns nil if OAuth is not supported or discovery fails.
func DiscoverOAuth(ctx context.Context, serverURL string) (*Config, error) {
	slog.Info("Discovering OAuth 2.0 configuration", "url", serverURL)
	parsed, err := url.Parse(serverURL)
	if err != nil {
		return nil, fmt.Errorf("invalid oauth server URL: %w", err)
	}

	// Build the well-known URL according to RFC 8414
	wellKnownURL := fmt.Sprintf("%s://%s/.well-known/oauth-authorization-server", parsed.Scheme, parsed.Host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnownURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth discovery request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		slog.Debug("OAuth discovery request failed", "error", err)
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		slog.Debug("OAuth well-known endpoint not found", "url", wellKnownURL)
		return nil, nil // No OAuth metadata, server doesn't support OAuth discovery
	}

	if resp.StatusCode != http.StatusOK {
		slog.Debug("OAuth discovery returned non-OK status", "status", resp.StatusCode)
		return nil, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read discovery response: %w", err)
	}

	var discovery discoveryResponse
	if err = json.Unmarshal(body, &discovery); err != nil {
		slog.Debug("Failed to parse OAuth metadata", "error", err)
		return nil, nil // Invalid metadata, treat as no OAuth
	}

	if err = validateDiscoveryResponse(&discovery, parsed.Scheme, parsed.Host); err != nil {
		slog.Debug("OAuth metadata validation failed", "error", err)
		return nil, nil
	}

	slog.Info("Discovered OAuth metadata successfully", "issuer", discovery.Issuer)
	slog.Debug("OAuth metadata discovered",
		"issuer", discovery.Issuer,
		"auth_endpoint", discovery.AuthorizationEndpoint,
		"registration_endpoint", discovery.RegistrationEndpoint,
		"token_endpoint", discovery.TokenEndpoint,
		"scopes_supported", strings.Join(discovery.ScopesSupported, ","),
	)

	return &Config{
		AuthURL:              discovery.AuthorizationEndpoint,
		TokenURL:             discovery.TokenEndpoint,
		Scopes:               discovery.ScopesSupported,
		RegistrationEndpoint: discovery.RegistrationEndpoint,
	}, nil
}
