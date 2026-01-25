package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// ClientRegistrationRequest represents a Dynamic Client Registration request (RFC 7591).
type ClientRegistrationRequest struct {
	// RedirectURIs is the list of allowed redirect URIs for this client.
	RedirectURIs []string `json:"redirect_uris"`
	// ClientName is the human-readable name of the client.
	ClientName string `json:"client_name,omitempty"`
	// TokenEndpointAuthMethod is the authentication method for the token endpoint.
	// For public clients, this should be "none".
	TokenEndpointAuthMethod string `json:"token_endpoint_auth_method,omitempty"`
	// GrantTypes is the list of OAuth grant types the client will use.
	GrantTypes []string `json:"grant_types,omitempty"`
	// ResponseTypes is the list of OAuth response types the client will use.
	ResponseTypes []string `json:"response_types,omitempty"`
	// Scope is the space-separated list of scopes the client is requesting.
	Scope string `json:"scope,omitempty"`
}

// ClientRegistrationResponse represents the response from client registration.
type ClientRegistrationResponse struct {
	// ClientID is the unique identifier for the registered client.
	ClientID string `json:"client_id"`
	// ClientSecret is the client secret (may be empty for public clients).
	ClientSecret string `json:"client_secret,omitempty"`
	// ClientIDIssuedAt is the time when the client_id was issued (Unix timestamp).
	ClientIDIssuedAt int64 `json:"client_id_issued_at,omitempty"`
	// ClientSecretExpiresAt is the time when the client_secret expires (Unix timestamp).
	// 0 means the secret does not expire.
	ClientSecretExpiresAt int64 `json:"client_secret_expires_at,omitempty"`
	// RegistrationAccessToken is used for reading/updating/deleting the client registration.
	RegistrationAccessToken string `json:"registration_access_token,omitempty"`
	// RegistrationClientURI is the URI to manage this client registration.
	RegistrationClientURI string `json:"registration_client_uri,omitempty"`

	// Echo back of request fields
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
}

// ClientCredentials holds the registered client credentials.
type ClientCredentials struct {
	ClientID                string `json:"client_id"`
	ClientSecret            string `json:"client_secret,omitempty"`
	RegistrationAccessToken string `json:"registration_access_token,omitempty"`
	RegistrationClientURI   string `json:"registration_client_uri,omitempty"`
}

// RegisterClient registers a new OAuth client with the authorization server.
// This implements RFC 7591 (OAuth 2.0 Dynamic Client Registration).
func RegisterClient(ctx context.Context, cfg Config) (*ClientCredentials, error) {
	if cfg.RegistrationEndpoint == "" {
		return nil, fmt.Errorf("registration endpoint is required")
	}
	if cfg.RedirectURI == "" {
		return nil, fmt.Errorf("redirect URI is required")
	}

	// Build registration request
	regReq := ClientRegistrationRequest{
		RedirectURIs:            []string{cfg.RedirectURI},
		ClientName:              "crush-oauth-client",
		TokenEndpointAuthMethod: "none", // Public client
		GrantTypes:              []string{"authorization_code", "refresh_token"},
		ResponseTypes:           []string{"code"},
	}

	if len(cfg.Scopes) > 0 {
		regReq.Scope = strings.Join(cfg.Scopes, " ")
	}

	slog.Debug("Registering OAuth client",
		"endpoint", cfg.RegistrationEndpoint,
	)

	body, err := json.Marshal(regReq)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize registration request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.RegistrationEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registration request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read registration response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return nil, fmt.Errorf("client registration failed: %s - %s", errResp.Error, errResp.ErrorDescription)
		}
		return nil, fmt.Errorf("client registration failed: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var regResp ClientRegistrationResponse
	if err = json.Unmarshal(respBody, &regResp); err != nil {
		return nil, fmt.Errorf("failed to parse registration response: %w", err)
	}

	slog.Info("OAuth client registered successfully",
		"client_id", regResp.ClientID,
	)

	return &ClientCredentials{
		ClientID:                regResp.ClientID,
		ClientSecret:            regResp.ClientSecret,
		RegistrationAccessToken: regResp.RegistrationAccessToken,
		RegistrationClientURI:   regResp.RegistrationClientURI,
	}, nil
}
