package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewTokenStore(t *testing.T) {
	t.Run("uses global data directory", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)

		store := NewTokenStore()
		require.NotNil(t, store)
		require.Equal(t, filepath.Join(tempDir, "mcp.json"), store.path)
	})
}

func TestTokenStore_Load(t *testing.T) {
	t.Run("returns nil when file does not exist", func(t *testing.T) {
		t.Setenv("CRUSH_GLOBAL_DATA", t.TempDir())
		store := NewTokenStore()

		loaded, err := store.Load("nonexistent")
		require.NoError(t, err)
		require.Nil(t, loaded)
	})

	t.Run("returns nil when entry does not exist", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)
		store := NewTokenStore()

		// Save one entry
		err := store.Save("other-mcp", &MCPOAuthData{AccessToken: "token"})
		require.NoError(t, err)

		// Load a different entry
		loaded, err := store.Load("nonexistent")
		require.NoError(t, err)
		require.Nil(t, loaded)
	})

	t.Run("returns error on invalid JSON", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)

		// Write invalid JSON
		mcpFile := filepath.Join(tempDir, "mcp.json")
		err := os.WriteFile(mcpFile, []byte("not valid json"), 0o600)
		require.NoError(t, err)

		store := NewTokenStore()
		loaded, err := store.Load("test")
		require.Error(t, err)
		require.Nil(t, loaded)
	})

	t.Run("loads all fields correctly", func(t *testing.T) {
		t.Setenv("CRUSH_GLOBAL_DATA", t.TempDir())
		store := NewTokenStore()

		data := &MCPOAuthData{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    3600,
			ExpiresAt:    1234567890,
			ClientID:     "client-id",
			ClientSecret: "client-secret",
		}
		err := store.Save("test-mcp", data)
		require.NoError(t, err)

		loaded, err := store.Load("test-mcp")
		require.NoError(t, err)
		require.NotNil(t, loaded)
		require.Equal(t, data.AccessToken, loaded.AccessToken)
		require.Equal(t, data.RefreshToken, loaded.RefreshToken)
		require.Equal(t, data.ExpiresIn, loaded.ExpiresIn)
		require.Equal(t, data.ExpiresAt, loaded.ExpiresAt)
		require.Equal(t, data.ClientID, loaded.ClientID)
		require.Equal(t, data.ClientSecret, loaded.ClientSecret)
	})
}

func TestTokenStore_Save(t *testing.T) {
	t.Run("creates file if not exists", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)
		store := NewTokenStore()

		err := store.Save("test-mcp", &MCPOAuthData{AccessToken: "token"})
		require.NoError(t, err)

		mcpFile := filepath.Join(tempDir, "mcp.json")
		_, err = os.Stat(mcpFile)
		require.NoError(t, err)
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		tempDir := t.TempDir()
		nestedDir := filepath.Join(tempDir, "nested", "path")
		t.Setenv("CRUSH_GLOBAL_DATA", nestedDir)
		store := NewTokenStore()

		err := store.Save("test-mcp", &MCPOAuthData{AccessToken: "token"})
		require.NoError(t, err)

		mcpFile := filepath.Join(nestedDir, "mcp.json")
		_, err = os.Stat(mcpFile)
		require.NoError(t, err)
	})

	t.Run("sets restrictive file permissions", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)
		store := NewTokenStore()

		err := store.Save("test-mcp", &MCPOAuthData{AccessToken: "token"})
		require.NoError(t, err)

		mcpFile := filepath.Join(tempDir, "mcp.json")
		info, err := os.Stat(mcpFile)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	})

	t.Run("preserves other entries when saving", func(t *testing.T) {
		t.Setenv("CRUSH_GLOBAL_DATA", t.TempDir())
		store := NewTokenStore()

		// Save first entry
		err := store.Save("mcp-1", &MCPOAuthData{AccessToken: "token-1"})
		require.NoError(t, err)

		// Save second entry
		err = store.Save("mcp-2", &MCPOAuthData{AccessToken: "token-2"})
		require.NoError(t, err)

		// Verify first entry still exists
		loaded, err := store.Load("mcp-1")
		require.NoError(t, err)
		require.Equal(t, "token-1", loaded.AccessToken)

		// Verify second entry exists
		loaded, err = store.Load("mcp-2")
		require.NoError(t, err)
		require.Equal(t, "token-2", loaded.AccessToken)
	})

	t.Run("updates existing entry", func(t *testing.T) {
		t.Setenv("CRUSH_GLOBAL_DATA", t.TempDir())
		store := NewTokenStore()

		// Save initial data
		err := store.Save("test-mcp", &MCPOAuthData{
			AccessToken:  "old-token",
			RefreshToken: "old-refresh",
			ClientID:     "client-id",
		})
		require.NoError(t, err)

		// Update with new token data
		err = store.Save("test-mcp", &MCPOAuthData{
			AccessToken:  "new-token",
			RefreshToken: "new-refresh",
			ClientID:     "client-id",
		})
		require.NoError(t, err)

		// Verify the update
		loaded, err := store.Load("test-mcp")
		require.NoError(t, err)
		require.Equal(t, "new-token", loaded.AccessToken)
		require.Equal(t, "new-refresh", loaded.RefreshToken)
		require.Equal(t, "client-id", loaded.ClientID)
	})

	t.Run("returns error on corrupted existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		t.Setenv("CRUSH_GLOBAL_DATA", tempDir)

		// Write invalid JSON
		mcpFile := filepath.Join(tempDir, "mcp.json")
		err := os.WriteFile(mcpFile, []byte("not valid json"), 0o600)
		require.NoError(t, err)

		store := NewTokenStore()
		err = store.Save("test-mcp", &MCPOAuthData{AccessToken: "token"})
		require.Error(t, err)
	})
}
