package utils

import (
	"fmt"
	"testing"
)

func TestReadCallbackPort(t *testing.T) {
	tests := []struct {
		name     string
		mockData []byte
		mockErr  error
		expected string
	}{
		{
			name:     "reads port from file",
			mockData: []byte("43210\n"),
			expected: "43210",
		},
		{
			name:     "trims whitespace",
			mockData: []byte("  12345  \n"),
			expected: "12345",
		},
		{
			name:     "returns empty for missing file",
			mockErr:  fmt.Errorf("no such file"),
			expected: "",
		},
		{
			name:     "returns empty for empty file",
			mockData: []byte(""),
			expected: "",
		},
		{
			name:     "returns empty for whitespace-only file",
			mockData: []byte("   \n"),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := readFileFunc
			defer func() { readFileFunc = orig }()

			readFileFunc = func(_ string) ([]byte, error) {
				if tt.mockErr != nil {
					return nil, tt.mockErr
				}
				return tt.mockData, nil
			}

			port := readCallbackPort()
			if port != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, port)
			}
		})
	}
}

func TestBuildOIDCArgs(t *testing.T) {
	tests := []struct {
		name             string
		noStore          bool
		callbackPort     string
		expectNoStore    bool
		expectFieldToken bool
		expectPort       bool
		expectCallback   bool
	}{
		{
			name:           "with store, no callback port",
			noStore:        false,
			callbackPort:   "",
			expectNoStore:  false,
			expectPort:     false,
			expectCallback: false,
		},
		{
			name:             "without store, no callback port",
			noStore:          true,
			callbackPort:     "",
			expectNoStore:    true,
			expectFieldToken: true,
			expectPort:       false,
			expectCallback:   false,
		},
		{
			name:           "with store, with callback port",
			noStore:        false,
			callbackPort:   "43210",
			expectNoStore:  false,
			expectPort:     true,
			expectCallback: true,
		},
		{
			name:             "without store, with callback port",
			noStore:          true,
			callbackPort:     "43210",
			expectNoStore:    true,
			expectFieldToken: true,
			expectPort:       true,
			expectCallback:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := buildOIDCArgs(tt.noStore, tt.callbackPort)

			argSet := map[string]bool{}
			for _, arg := range args {
				argSet[arg] = true
			}

			if !argSet["skip_browser=true"] {
				t.Errorf("expected skip_browser=true, got args: %v", args)
			}
			if !argSet["listenaddress=0.0.0.0"] {
				t.Errorf("expected listenaddress=0.0.0.0, got args: %v", args)
			}
			if !argSet["login"] {
				t.Errorf("expected login, got args: %v", args)
			}
			if !argSet["-method=oidc"] {
				t.Errorf("expected -method=oidc, got args: %v", args)
			}

			if tt.expectNoStore && !argSet["-no-store"] {
				t.Errorf("expected -no-store, got args: %v", args)
			}
			if !tt.expectNoStore && argSet["-no-store"] {
				t.Errorf("did not expect -no-store, got args: %v", args)
			}
			if tt.expectFieldToken && !argSet["-field=token"] {
				t.Errorf("expected -field=token, got args: %v", args)
			}
			if !tt.expectFieldToken && argSet["-field=token"] {
				t.Errorf("did not expect -field=token, got args: %v", args)
			}

			expectPortArg := fmt.Sprintf("port=%s", defaultVaultOIDCPort)
			expectCallbackArg := fmt.Sprintf("callbackport=%s", tt.callbackPort)

			if tt.expectPort && !argSet[expectPortArg] {
				t.Errorf("expected %s, got args: %v", expectPortArg, args)
			}
			if !tt.expectPort && argSet[expectPortArg] {
				t.Errorf("did not expect %s, got args: %v", expectPortArg, args)
			}
			if tt.expectCallback && !argSet[expectCallbackArg] {
				t.Errorf("expected %s, got args: %v", expectCallbackArg, args)
			}
			if !tt.expectCallback && argSet[expectCallbackArg] {
				t.Errorf("did not expect %s, got args: %v", expectCallbackArg, args)
			}
		})
	}
}

func TestIsTokenFileError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "rename error",
			err:      fmt.Errorf("rename /root/.vault-token.tmp /root/.vault-token: device or resource busy"),
			expected: true,
		},
		{
			name:     "device or resource busy",
			err:      fmt.Errorf("device or resource busy"),
			expected: true,
		},
		{
			name:     "read-only file system",
			err:      fmt.Errorf("read-only file system"),
			expected: true,
		},
		{
			name:     "permission denied",
			err:      fmt.Errorf("permission denied"),
			expected: true,
		},
		{
			name:     "auth timeout",
			err:      fmt.Errorf("context deadline exceeded"),
			expected: false,
		},
		{
			name:     "network error",
			err:      fmt.Errorf("dial tcp: connection refused"),
			expected: false,
		},
		{
			name:     "generic vault error",
			err:      fmt.Errorf("Error making API request"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isTokenFileError(tt.err)
			if result != tt.expected {
				t.Errorf("isTokenFileError(%q) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
