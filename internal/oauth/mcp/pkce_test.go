package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratePKCE(t *testing.T) {
	verifier, challenge := generatePKCE()

	// Verifier should be 43 characters (32 bytes base64url encoded)
	require.Len(t, verifier, 43)

	// Challenge should be 43 characters (32 bytes SHA256 -> base64url encoded)
	require.Len(t, challenge, 43)

	// Verifier and challenge should be different
	require.NotEqual(t, verifier, challenge)

	// Multiple calls should generate different values
	verifier2, challenge2 := generatePKCE()
	require.NotEqual(t, verifier, verifier2)
	require.NotEqual(t, challenge, challenge2)
}
