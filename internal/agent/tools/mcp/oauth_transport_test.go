package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/charmbracelet/crush/internal/oauth"
	mcpoauth "github.com/charmbracelet/crush/internal/oauth/mcp"
	"github.com/stretchr/testify/require"
)

func validConfig() mcpoauth.Config {
	return mcpoauth.Config{
		ClientID: "test-client-id",
		AuthURL:  "https://example.com/auth",
		TokenURL: "https://example.com/token",
	}
}

func validToken() *oauth.Token {
	return &oauth.Token{
		AccessToken:  "valid-access-token",
		RefreshToken: "valid-refresh-token",
		ExpiresIn:    3600,
		ExpiresAt:    time.Now().Add(time.Hour).Unix(),
	}
}

func expiredTokenNoRefresh() *oauth.Token {
	return &oauth.Token{
		AccessToken: "expired-access-token",
		ExpiresIn:   3600,
		ExpiresAt:   time.Now().Add(-time.Hour).Unix(),
	}
}

// newTestStore creates a TokenStore for testing with a temp directory.
func newTestStore(t *testing.T) *TokenStore {
	t.Helper()
	t.Setenv("CRUSH_GLOBAL_DATA", t.TempDir())
	return NewTokenStore()
}

// saveTestToken saves an oauth.Token to the store using the new MCPOAuthData format.
func saveTestToken(t *testing.T, store *TokenStore, name string, token *oauth.Token) {
	t.Helper()
	data := &MCPOAuthData{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		ExpiresIn:    token.ExpiresIn,
		ExpiresAt:    token.ExpiresAt,
	}
	err := store.Save(name, data)
	require.NoError(t, err)
}

// loadTestToken loads a token from the store and converts to oauth.Token.
func loadTestToken(t *testing.T, store *TokenStore, name string) *oauth.Token {
	t.Helper()
	data, err := store.Load(name)
	require.NoError(t, err)
	if data == nil || data.AccessToken == "" {
		return nil
	}
	return &oauth.Token{
		AccessToken:  data.AccessToken,
		RefreshToken: data.RefreshToken,
		ExpiresIn:    data.ExpiresIn,
		ExpiresAt:    data.ExpiresAt,
	}
}

func TestNewMCPTokenProvider(t *testing.T) {
	t.Run("requires non-nil store", func(t *testing.T) {
		_, err := NewOAuthTokenProvider("test", validConfig(), nil)
		require.Error(t, err)
	})

	t.Run("validates config", func(t *testing.T) {
		store := newTestStore(t)
		_, err := NewOAuthTokenProvider("test", mcpoauth.Config{}, store)
		require.Error(t, err)
	})

	t.Run("creates provider with valid inputs", func(t *testing.T) {
		store := newTestStore(t)
		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)
		require.NotNil(t, provider)
	})
}

func TestMCPTokenProvider_EnsureToken(t *testing.T) {
	t.Run("returns cached valid token", func(t *testing.T) {
		store := newTestStore(t)
		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		cachedToken := validToken()
		provider.token = cachedToken

		token, err := provider.EnsureToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, cachedToken, token)
	})

	t.Run("loads valid token from store", func(t *testing.T) {
		store := newTestStore(t)
		storedToken := validToken()
		saveTestToken(t, store, "test", storedToken)

		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		token, err := provider.EnsureToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, storedToken.AccessToken, token.AccessToken)
	})

	t.Run("uses authFunc when no valid token", func(t *testing.T) {
		store := newTestStore(t)
		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		expected := validToken()
		expected.AccessToken = "new-auth-token"
		provider.SetAuthFunc(func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error) {
			return expected, nil
		})

		token, err := provider.EnsureToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, expected.AccessToken, token.AccessToken)
	})

	t.Run("token saved and retrievable from store", func(t *testing.T) {
		store := newTestStore(t)
		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		expected := validToken()
		expected.AccessToken = "new-auth-token"
		provider.SetAuthFunc(func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error) {
			return expected, nil
		})

		_, err = provider.EnsureToken(context.Background())
		require.NoError(t, err)

		// Verify token was saved to store
		loaded := loadTestToken(t, store, "test")
		require.NotNil(t, loaded)
		require.Equal(t, expected.AccessToken, loaded.AccessToken)
	})

	t.Run("returns error when no token and no authFunc", func(t *testing.T) {
		store := newTestStore(t)
		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		token, err := provider.EnsureToken(context.Background())
		require.Error(t, err)
		require.Nil(t, token)
		require.Contains(t, err.Error(), "no valid token available")
	})

	t.Run("falls back to authFunc when store has expired token without refresh", func(t *testing.T) {
		store := newTestStore(t)
		saveTestToken(t, store, "test", expiredTokenNoRefresh())

		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		expected := validToken()
		expected.AccessToken = "fresh-from-auth"
		provider.SetAuthFunc(func(ctx context.Context, cfg mcpoauth.Config) (*oauth.Token, error) {
			return expected, nil
		})

		token, err := provider.EnsureToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, expected.AccessToken, token.AccessToken)
	})

	t.Run("uses non-expired token before others", func(t *testing.T) {
		store := newTestStore(t)
		storedToken := validToken()
		saveTestToken(t, store, "test", storedToken)

		provider, err := NewOAuthTokenProvider("test", validConfig(), store)
		require.NoError(t, err)

		// First call loads from store
		_, err = provider.EnsureToken(context.Background())
		require.NoError(t, err)

		// Overwrite store with a different token
		differentToken := validToken()
		differentToken.AccessToken = "different-token"
		saveTestToken(t, store, "test", differentToken)

		// Second call should use cached token, not the new one in store
		token, err := provider.EnsureToken(context.Background())
		require.NoError(t, err)
		require.Equal(t, storedToken.AccessToken, token.AccessToken)
	})
}
