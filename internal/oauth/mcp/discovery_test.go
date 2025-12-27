package mcp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateDiscoveryResponse(t *testing.T) {
	tests := []struct {
		name    string
		resp    *discoveryResponse
		scheme  string
		host    string
		wantErr bool
	}{
		{
			name: "valid response",
			resp: &discoveryResponse{
				Issuer:                 "https://example.com",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme: "https",
			host:   "example.com",
		},
		{
			name: "valid response with issuer path",
			resp: &discoveryResponse{
				Issuer:                 "https://example.com/issuers/mcp",
				AuthorizationEndpoint:  "https://example.com/issuers/mcp/authorize",
				TokenEndpoint:          "https://example.com/issuers/mcp/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme: "https",
			host:   "example.com",
		},
		{
			name: "missing issuer",
			resp: &discoveryResponse{
				Issuer:                 "",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "missing authorization_endpoint",
			resp: &discoveryResponse{
				Issuer:                 "https://example.com",
				AuthorizationEndpoint:  "",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "missing token_endpoint",
			resp: &discoveryResponse{
				Issuer:                 "https://example.com",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "",
				ResponseTypesSupported: []string{"code"},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "missing response_types_supported",
			resp: &discoveryResponse{
				Issuer:                "https://example.com",
				AuthorizationEndpoint: "https://example.com/authorize",
				TokenEndpoint:         "https://example.com/token",
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "empty response_types_supported",
			resp: &discoveryResponse{
				Issuer:                 "https://example.com",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "issuer host mismatch",
			resp: &discoveryResponse{
				Issuer:                 "https://evil.com",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
		{
			name: "issuer scheme mismatch",
			resp: &discoveryResponse{
				Issuer:                 "http://example.com",
				AuthorizationEndpoint:  "https://example.com/authorize",
				TokenEndpoint:          "https://example.com/token",
				ResponseTypesSupported: []string{"code"},
			},
			scheme:  "https",
			host:    "example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDiscoveryResponse(tt.resp, tt.scheme, tt.host)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
