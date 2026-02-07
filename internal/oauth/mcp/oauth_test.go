package mcp

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthorizeURL(t *testing.T) {
	cfg := Config{
		ClientID:    "test-client-id",
		AuthURL:     "https://auth.example.com/authorize",
		TokenURL:    "https://auth.example.com/token",
		Scopes:      []string{"read", "write"},
		RedirectURI: "http://localhost:8080/callback",
	}

	state := "test-state"
	challenge := "test-challenge"

	result, err := authorizeURL(cfg, state, challenge)
	require.NoError(t, err)

	parsed, err := url.Parse(result)
	require.NoError(t, err)

	// Verify base URL
	require.Equal(t, "https", parsed.Scheme)
	require.Equal(t, "auth.example.com", parsed.Host)
	require.Equal(t, "/authorize", parsed.Path)

	// Expected query parameters - add new params here
	expected := map[string]string{
		"client_id":             "test-client-id",
		"redirect_uri":          "http://localhost:8080/callback",
		"response_type":         "code",
		"state":                 "test-state",
		"scope":                 "read write",
		"code_challenge":        "test-challenge",
		"code_challenge_method": "S256",
	}

	query := parsed.Query()

	// Verify count matches - catches forgotten or extra params
	require.Equal(t, len(expected), len(query),
		"query parameter count mismatch: expected %d, got %d", len(expected), len(query))

	// Verify each expected parameter
	for key, want := range expected {
		require.Equal(t, want, query.Get(key), "parameter %q mismatch", key)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		wantURI string // expected RedirectURI after validation
	}{
		{
			name: "valid config with client_id gets default redirect URI",
			config: Config{
				ClientID: "test-client",
				AuthURL:  "https://auth.example.com/authorize",
				TokenURL: "https://auth.example.com/token",
			},
			wantURI: DefaultRedirectURI,
		},
		{
			name: "valid config with registration endpoint",
			config: Config{
				RegistrationEndpoint: "https://auth.example.com/register",
				AuthURL:              "https://auth.example.com/authorize",
				TokenURL:             "https://auth.example.com/token",
			},
			wantURI: DefaultRedirectURI,
		},
		{
			name: "valid config with custom redirect URI",
			config: Config{
				ClientID:    "test-client",
				RedirectURI: "http://localhost:9000/oauth/cb",
			},
			wantURI: "http://localhost:9000/oauth/cb",
		},
		{
			name: "valid config with 127.0.0.1",
			config: Config{
				ClientID:    "test-client",
				RedirectURI: "http://127.0.0.1:8080/callback",
			},
			wantURI: "http://127.0.0.1:8080/callback",
		},
		{
			name: "missing client_id and registration endpoint",
			config: Config{
				AuthURL:  "https://auth.example.com/authorize",
				TokenURL: "https://auth.example.com/token",
			},
			wantErr: true,
		},
		{
			name: "https redirect URI rejected",
			config: Config{
				ClientID:    "test-client",
				RedirectURI: "https://localhost:8080/callback",
			},
			wantErr: true,
		},
		{
			name: "non-localhost redirect URI rejected",
			config: Config{
				ClientID:    "test-client",
				RedirectURI: "http://example.com:8080/callback",
			},
			wantErr: true,
		},
		{
			name: "IPv6 redirect URI rejected",
			config: Config{
				ClientID:    "test-client",
				RedirectURI: "http://[::1]:8080/callback",
			},
			wantErr: true,
		},
		{
			name: "invalid auth_url rejected",
			config: Config{
				ClientID: "test-client",
				AuthURL:  "://invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid token_url rejected",
			config: Config{
				ClientID: "test-client",
				TokenURL: "://invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid registration_endpoint rejected",
			config: Config{
				RegistrationEndpoint: "://invalid",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantURI, tt.config.RedirectURI)
		})
	}
}
