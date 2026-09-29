package rhobs

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// setRhobsTestEnv restores the original environment on cleanup and treats empty
// test values as unset. Tests for explicitly empty variables use t.Setenv directly.
func setRhobsTestEnv(t *testing.T, clientID, clientSecret string) {
	t.Helper()
	for key, value := range map[string]string{
		rhobsClientIDEnvVar: clientID, rhobsClientSecretEnvVar: clientSecret,
	} {
		t.Setenv(key, value)
		if value == "" {
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// setRhobsTestConfig isolates tests from the user's osdctl configuration.
func setRhobsTestConfig(t *testing.T, clientID, clientSecret string) {
	t.Helper()
	original := readRhobsConfig
	t.Cleanup(func() { readRhobsConfig = original })
	readRhobsConfig = func(keys ...string) (map[string]string, error) {
		if len(keys) != 2 || keys[0] != rhobsClientIDConfigKey || keys[1] != rhobsClientSecretConfigKey {
			t.Fatalf("unexpected config keys: %v", keys)
		}
		return map[string]string{
			rhobsClientIDConfigKey:     clientID,
			rhobsClientSecretConfigKey: clientSecret,
		}, nil
	}
}

// TestGetTokenProvider_CredentialResolution checks precedence, mixed sources,
// complete pairs, and incomplete pairs without using live credentials.
func TestGetTokenProvider_CredentialResolution(t *testing.T) {
	for _, tt := range []struct {
		name         string
		configID     string
		configSecret string
		envID        string
		envSecret    string
		wantID       string
		wantSecret   string
		wantErr      bool
	}{
		{name: "config pair", configID: "config-id", configSecret: "config-secret", wantID: "config-id", wantSecret: "config-secret"},
		{name: "env pair", envID: "env-id", envSecret: "env-secret", wantID: "env-id", wantSecret: "env-secret"},
		{name: "env overrides config", configID: "config-id", configSecret: "config-secret", envID: "env-id", envSecret: "env-secret", wantID: "env-id", wantSecret: "env-secret"},
		{name: "env ID and config secret", configID: "config-id", configSecret: "config-secret", envID: "env-id", wantID: "env-id", wantSecret: "config-secret"},
		{name: "config ID and env secret", configID: "config-id", configSecret: "config-secret", envSecret: "env-secret", wantID: "config-id", wantSecret: "env-secret"},
		{name: "only config ID", configID: "config-id", wantID: "config-id", wantErr: true},
		{name: "only config secret", configSecret: "config-secret", wantSecret: "config-secret", wantErr: true},
		{name: "only env ID", envID: "env-id", wantID: "env-id", wantErr: true},
		{name: "only env secret", envSecret: "env-secret", wantSecret: "env-secret", wantErr: true},
		{name: "no credentials"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setRhobsTestEnv(t, tt.envID, tt.envSecret)
			setRhobsTestConfig(t, tt.configID, tt.configSecret)

			id, secret, err := providedClientCredentials()
			if err != nil {
				t.Fatal(err)
			}
			if id != tt.wantID || secret != tt.wantSecret {
				t.Fatal("resolved credentials do not match expected sources")
			}
			hasCredentials, err := hasProvidedClientCredentials()
			if err != nil {
				t.Fatal(err)
			}
			if want := tt.wantID != "" || tt.wantSecret != ""; hasCredentials != want {
				t.Fatalf("hasProvidedClientCredentials = %v, want %v", hasCredentials, want)
			}
			if !hasCredentials {
				return // The MCP tests cover the Vault precheck when no credentials resolve.
			}

			fetcher := &RhobsFetcher{ocmEnvName: "production"}
			provider, err := fetcher.getTokenProvider()
			if tt.wantErr {
				if err == nil || !strings.Contains(err.Error(), "must be provided together") {
					t.Fatalf("expected incomplete-pair error, got %v", err)
				}
				return
			}
			if err != nil || provider == nil {
				t.Fatalf("expected a token provider, got %v", err)
			}
		})
	}
}

// TestProvidedClientCredentials_ConfigErrors verifies that a missing file permits
// fallback, unreadable config is reported, and complete env credentials skip reading.
func TestProvidedClientCredentials_ConfigErrors(t *testing.T) {
	for _, tt := range []struct {
		name      string
		readErr   error
		envID     string
		envSecret string
		wantRead  bool
		wantErr   bool
	}{
		{name: "missing config", readErr: os.ErrNotExist, wantRead: true},
		{name: "unreadable config", readErr: os.ErrPermission, wantRead: true, wantErr: true},
		{name: "invalid config", readErr: errors.New("invalid YAML"), wantRead: true, wantErr: true},
		{name: "complete env skips config", readErr: os.ErrPermission, envID: "env-id", envSecret: "env-secret"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setRhobsTestEnv(t, tt.envID, tt.envSecret)
			original := readRhobsConfig
			t.Cleanup(func() { readRhobsConfig = original })
			called := false
			readRhobsConfig = func(_ ...string) (map[string]string, error) {
				called = true
				return nil, tt.readErr
			}
			_, _, err := providedClientCredentials()
			if (err != nil) != tt.wantErr || called != tt.wantRead {
				t.Fatalf("read called = %v, error = %v; want read = %v, error = %v", called, err, tt.wantRead, tt.wantErr)
			}
			if tt.wantErr && !errors.Is(err, tt.readErr) {
				t.Fatalf("config error was not preserved: %v", err)
			}
		})
	}
}

// TestProvidedClientCredentials_EmptyEnv verifies that explicitly empty variables
// fail before config is read or Vault is consulted, including when both are empty.
func TestProvidedClientCredentials_EmptyEnv(t *testing.T) {
	for _, tt := range []struct {
		name      string
		env       map[string]string
		wantError string
	}{
		{name: "empty ID", env: map[string]string{rhobsClientIDEnvVar: ""}, wantError: rhobsClientIDEnvVar},
		{name: "empty secret", env: map[string]string{rhobsClientSecretEnvVar: ""}, wantError: rhobsClientSecretEnvVar},
		{name: "both empty", env: map[string]string{rhobsClientIDEnvVar: "", rhobsClientSecretEnvVar: ""}, wantError: rhobsClientIDEnvVar},
		{name: "empty ID with secret", env: map[string]string{rhobsClientIDEnvVar: "", rhobsClientSecretEnvVar: "test-secret"}, wantError: rhobsClientIDEnvVar},
		{name: "ID with empty secret", env: map[string]string{rhobsClientIDEnvVar: "test-id", rhobsClientSecretEnvVar: ""}, wantError: rhobsClientSecretEnvVar},
	} {
		t.Run(tt.name, func(t *testing.T) {
			setRhobsTestEnv(t, "", "")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			original := readRhobsConfig
			t.Cleanup(func() { readRhobsConfig = original })
			readRhobsConfig = func(_ ...string) (map[string]string, error) {
				t.Fatal("explicitly empty environment variable must not fall back to config")
				return nil, nil
			}
			fetcher := &RhobsFetcher{ocmEnvName: "production"}
			provider, err := fetcher.getTokenProvider()
			want := tt.wantError + " is set but empty"
			if provider != nil || err == nil || err.Error() != want {
				t.Fatalf("expected no provider and error %q, got %v", want, err)
			}
			if _, err := hasProvidedClientCredentials(); err == nil || err.Error() != want {
				t.Fatalf("expected precheck error %q, got %v", want, err)
			}
		})
	}
}

// TestRhobsCredentialFlagsRemoved verifies that secrets cannot be supplied as CLI flags.
func TestRhobsCredentialFlagsRemoved(t *testing.T) {
	original := commonOptions
	t.Cleanup(func() { commonOptions = original })
	for _, flag := range []string{"--client-id", "--client-secret"} {
		for _, path := range [][]string{nil, {"logs"}, {"mcp", "server"}} {
			cmd, _, err := NewCmdRhobs().Find(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.ParseFlags([]string{flag, "dummy"}); err == nil || !strings.Contains(err.Error(), "unknown flag: "+flag) {
				t.Fatalf("expected %s to be rejected for %v, got %v", flag, path, err)
			}
		}
	}
}
