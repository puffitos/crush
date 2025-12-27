package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/oauth"
	"github.com/stretchr/testify/require"
)

func TestStartAuthFlow(t *testing.T) {
	t.Run("successful flow", func(t *testing.T) {
		code := "test-auth-code"
		grantType := "authorization_code"
		accessToken := "token"
		refreshToken := "refresh-token"
		expiration := 3600

		// Create a mock token endpoint
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "POST", r.Method)
			require.NoError(t, r.ParseForm())
			require.Equal(t, grantType, r.FormValue("grant_type"))
			require.Equal(t, code, r.FormValue("code"))
			require.NotEmpty(t, r.FormValue("code_verifier")) // PKCE

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  accessToken,
				"refresh_token": refreshToken,
				"expires_in":    expiration,
				"token_type":    "Bearer",
			})
		}))
		defer tokenServer.Close()

		cfg := Config{
			ClientID:    "test-client",
			AuthURL:     "http://localhost:19999/authorize", // Not actually called
			TokenURL:    tokenServer.URL,
			RedirectURI: "http://localhost:0/callback", // Use random port
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var capturedAuthURL string
		opts := AuthFlowOptions{
			Timeout:     5 * time.Second,
			OpenBrowser: false,
			OnAuthURL: func(authURL string) {
				capturedAuthURL = authURL
			},
		}

		// Run StartAuthFlow in a goroutine since it blocks waiting for callback
		type result struct {
			token *oauth.Token
			err   error
		}
		done := make(chan result, 1)

		go func() {
			token, err := StartAuthFlow(ctx, cfg, opts)
			done <- result{token, err}
		}()

		// Wait for the auth URL to be generated
		require.Eventually(t, func() bool {
			return capturedAuthURL != ""
		}, 2*time.Second, 10*time.Millisecond)

		// Parse the auth URL to extract state and redirect_uri
		authURL, err := url.Parse(capturedAuthURL)
		require.NoError(t, err)
		state := authURL.Query().Get("state")
		redirectURI := authURL.Query().Get("redirect_uri")
		require.NotEmpty(t, state)
		require.NotEmpty(t, redirectURI)

		// Simulate browser redirect to callback with code
		callbackURL := fmt.Sprintf("%s?code=%s&state=%s", redirectURI, code, state)
		resp, err := http.Get(callbackURL)
		require.NoError(t, err)
		_ = resp.Body.Close()

		// Wait for flow to complete
		select {
		case res := <-done:
			require.NoError(t, res.err)
			require.NotNil(t, res.token)
			require.Equal(t, accessToken, res.token.AccessToken)
			require.Equal(t, refreshToken, res.token.RefreshToken)
			require.Equal(t, expiration, res.token.ExpiresIn)
		case <-time.After(3 * time.Second):
			t.Fatal("timeout waiting for auth flow to complete")
		}
	})

	t.Run("timeout handling", func(t *testing.T) {
		cfg := Config{
			ClientID: "test-client",
			AuthURL:  "http://localhost:19999/authorize",
			TokenURL: "http://localhost:19999/token",
		}

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		opts := AuthFlowOptions{
			Timeout:     100 * time.Millisecond,
			OpenBrowser: false,
		}

		// The flow should timeout since no one is completing the auth
		token, err := StartAuthFlow(ctx, cfg, opts)
		require.Error(t, err)
		require.Nil(t, token)
	})
}

func TestGenerateState(t *testing.T) {

	// 32 hex chars
	state1 := generateState()
	require.Len(t, state1, 32)

	state2 := generateState()
	require.Len(t, state2, 32)

	// States should be unique
	require.NotEqual(t, state1, state2)
}

func TestParseRedirectURI(t *testing.T) {
	// Note: parseRedirectURI expects already-validated URIs from Config.Validate().
	// Validation tests (invalid scheme, non-localhost, etc.) are in TestConfigValidate.
	tests := []struct {
		name        string
		redirectURI string
		wantPort    int
		wantPath    string
	}{
		{
			name:        "localhost with port and path",
			redirectURI: "http://localhost:8080/callback",
			wantPort:    8080,
			wantPath:    "/callback",
		},
		{
			name:        "localhost with custom path",
			redirectURI: "http://localhost:9000/oauth/cb",
			wantPort:    9000,
			wantPath:    "/oauth/cb",
		},
		{
			name:        "127.0.0.1 with port",
			redirectURI: "http://127.0.0.1:3000/callback",
			wantPort:    3000,
			wantPath:    "/callback",
		},
		{
			name:        "localhost without port uses random",
			redirectURI: "http://localhost/callback",
			wantPort:    0,
			wantPath:    "/callback",
		},
		{
			name:        "localhost without path uses default",
			redirectURI: "http://localhost:8080",
			wantPort:    8080,
			wantPath:    "/callback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, path := parseRedirectURI(tt.redirectURI)
			require.Equal(t, tt.wantPort, port)
			require.Equal(t, tt.wantPath, path)
		})
	}
}
