package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/charmbracelet/crush/internal/home"
)

// MCPOAuthData holds OAuth tokens and client credentials for an MCP server.
type MCPOAuthData struct {
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	ExpiresIn    int    `json:"expires_in,omitempty"`
	ExpiresAt    int64  `json:"expires_at,omitempty"`
	ClientID     string `json:"client_id,omitempty"`
	ClientSecret string `json:"client_secret,omitempty"`
}

// TokenStore handles persistence of MCP OAuth data globally.
// Data is stored in ~/.local/share/crush/mcp.json (or platform equivalent).
type TokenStore struct {
	path string
	mu   sync.RWMutex
}

// NewTokenStore creates a new TokenStore using the global data directory.
func NewTokenStore() *TokenStore {
	dataDir := os.Getenv("CRUSH_GLOBAL_DATA")
	if dataDir == "" {
		dataDir = filepath.Join(home.Dir(), ".local", "share", "crush")
	}
	return &TokenStore{
		path: filepath.Join(dataDir, "mcp.json"),
	}
}

// Load returns the OAuth data for an MCP server, or nil if not found.
// Returns an error if the file exists but cannot be read or parsed.
func (s *TokenStore) Load(mcpName string) (*MCPOAuthData, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read MCP OAuth file: %w", err)
	}

	var store map[string]*MCPOAuthData
	if err = json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("failed to parse MCP OAuth file: %w", err)
	}

	return store[mcpName], nil
}

// Save persists the OAuth data for an MCP server.
func (s *TokenStore) Save(mcpName string, oauthData *MCPOAuthData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Load existing data
	store := make(map[string]*MCPOAuthData)
	data, err := os.ReadFile(s.path)
	if err == nil {
		// File exists, parse it
		if err = json.Unmarshal(data, &store); err != nil {
			return fmt.Errorf("failed to parse existing MCP OAuth file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read MCP OAuth file: %w", err)
	}

	// Update the entry
	store[mcpName] = oauthData

	// Ensure directory exists
	if err = os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("failed to create MCP OAuth directory: %w", err)
	}

	// Write back
	newData, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal MCP OAuth data: %w", err)
	}

	if err = os.WriteFile(s.path, newData, 0o600); err != nil {
		return fmt.Errorf("failed to write MCP OAuth file: %w", err)
	}

	return nil
}
