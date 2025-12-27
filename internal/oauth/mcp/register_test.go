package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRegisterClient(t *testing.T) {
	t.Run("successful registration", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "POST", r.Method)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req ClientRegistrationRequest
			err := json.NewDecoder(r.Body).Decode(&req)
			require.NoError(t, err)

			require.Equal(t, []string{"http://localhost:19876/callback"}, req.RedirectURIs)
			require.Equal(t, "crush-oauth-client", req.ClientName)
			require.Equal(t, "none", req.TokenEndpointAuthMethod)
			require.Contains(t, req.GrantTypes, "authorization_code")
			require.Contains(t, req.GrantTypes, "refresh_token")

			resp := ClientRegistrationResponse{
				ClientID:     "registered-client-id",
				ClientSecret: "",
				ClientName:   req.ClientName,
				RedirectURIs: req.RedirectURIs,
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		cfg := Config{
			RegistrationEndpoint: server.URL,
			RedirectURI:          "http://localhost:19876/callback",
			Scopes:               []string{"openid", "profile"},
		}
		creds, err := RegisterClient(context.Background(), cfg)
		require.NoError(t, err)
		require.NotNil(t, creds)
		require.Equal(t, "registered-client-id", creds.ClientID)
		require.Empty(t, creds.ClientSecret) // Public client
	})

	t.Run("server error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		cfg := Config{
			RegistrationEndpoint: server.URL,
			RedirectURI:          "http://localhost:19876/callback",
		}
		creds, err := RegisterClient(context.Background(), cfg)
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("empty registration endpoint", func(t *testing.T) {
		cfg := Config{
			RedirectURI: "http://localhost:19876/callback",
		}
		creds, err := RegisterClient(context.Background(), cfg)
		require.Error(t, err)
		require.Nil(t, creds)
		require.Contains(t, err.Error(), "registration endpoint is required")
	})

	t.Run("empty redirect URI", func(t *testing.T) {
		cfg := Config{
			RegistrationEndpoint: "https://example.com/register",
		}
		creds, err := RegisterClient(context.Background(), cfg)
		require.Error(t, err)
		require.Nil(t, creds)
		require.Contains(t, err.Error(), "redirect URI is required")
	})
}

func TestConfig_SupportsDynamicRegistration(t *testing.T) {
	t.Run("with registration endpoint", func(t *testing.T) {
		cfg := &Config{
			RegistrationEndpoint: "https://example.com/register",
		}
		require.True(t, cfg.SupportsDynamicRegistration())
	})

	t.Run("without registration endpoint", func(t *testing.T) {
		cfg := &Config{}
		require.False(t, cfg.SupportsDynamicRegistration())
	})
}
