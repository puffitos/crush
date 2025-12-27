// Package mcp provides OAuth 2.0 functionality for MCP server authentication.
package mcp

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// generatePKCE generates a PKCE code verifier and code challenge.
// The verifier is a cryptographically random string, and the challenge
// is its SHA256 hash encoded in base64url format. If the generation fails,
// the function panics.
func generatePKCE() (verifier, challenge string) {
	// Generate a 32-byte random verifier (will be 43 characters in base64url)
	verifierBytes := make([]byte, 32)
	_, _ = rand.Read(verifierBytes)
	verifier = base64.RawURLEncoding.EncodeToString(verifierBytes)

	// Generate the challenge using SHA256
	h := sha256.New()
	h.Write([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return verifier, challenge
}
