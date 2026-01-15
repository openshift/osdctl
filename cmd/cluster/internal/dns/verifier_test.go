package dns

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockResolver is a mock implementation of the Resolver interface
type MockResolver struct {
	mock.Mock
}

func (m *MockResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	args := m.Called(ctx, host)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockResolver) LookupCNAME(ctx context.Context, name string) (string, error) {
	args := m.Called(ctx, name)
	return args.String(0), args.Error(1)
}

func TestVerifyARecord(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		resolverIPs    []string
		resolverErr    error
		expectedStatus VerifyResultStatus
		expectedName   string
		expectedIPs    []string
		expectedError  string
	}{
		{
			name:           "successful A record lookup",
			path:           "https://console-openshift-console.apps.example.com",
			resolverIPs:    []string{"192.168.1.1", "192.168.1.2"},
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusPass,
			expectedName:   "console-openshift-console.apps.example.com",
			expectedIPs:    []string{"192.168.1.1", "192.168.1.2"},
			expectedError:  "",
		},
		{
			name:           "successful A record lookup without scheme",
			path:           "api.example.com",
			resolverIPs:    []string{"10.0.0.1"},
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusPass,
			expectedName:   "api.example.com",
			expectedIPs:    []string{"10.0.0.1"},
			expectedError:  "",
		},
		{
			name:           "DNS lookup failure",
			path:           "https://nonexistent.example.com",
			resolverIPs:    nil,
			resolverErr:    errors.New("no such host"),
			expectedStatus: VerifyResultStatusFail,
			expectedName:   "nonexistent.example.com",
			expectedIPs:    nil,
			expectedError:  "no such host",
		},
		{
			name:           "invalid URL",
			path:           "://invalid-url",
			resolverIPs:    nil,
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusFail,
			expectedName:   "://invalid-url",
			expectedIPs:    nil,
			expectedError:  "invalid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockResolver := new(MockResolver)

			// Only set up the mock if we have a valid URL
			if tt.expectedName != "://invalid-url" {
				mockResolver.On("LookupHost", mock.Anything, tt.expectedName).Return(tt.resolverIPs, tt.resolverErr)
			}

			verifier := &DefaultVerifier{
				cfg: DefaultVerifierConfig{
					Timeout:  10 * time.Second,
					Resolver: mockResolver,
				},
			}

			ctx := context.Background()
			result := verifier.VerifyARecord(ctx, tt.path)

			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, RecordTypeA, result.Type)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedIPs, result.ResolvedIPs)
			if tt.expectedError != "" {
				assert.Contains(t, result.ErrorMessage, tt.expectedError)
			}

			if tt.expectedName != "://invalid-url" {
				mockResolver.AssertExpectations(t)
			}
		})
	}
}

func TestVerifyCNAMERecord(t *testing.T) {
	tests := []struct {
		name           string
		dnsName        string
		expectedTarget string
		resolverCNAME  string
		resolverErr    error
		expectedStatus VerifyResultStatus
		expectedError  string
	}{
		{
			name:           "successful CNAME lookup without expected target",
			dnsName:        "apps.rosa.example.com",
			expectedTarget: "",
			resolverCNAME:  "cluster.example.com.",
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusPass,
			expectedError:  "",
		},
		{
			name:           "successful CNAME lookup with matching target",
			dnsName:        "apps.rosa.example.com",
			expectedTarget: "cluster.example.com",
			resolverCNAME:  "cluster.example.com.",
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusPass,
			expectedError:  "",
		},
		{
			name:           "successful CNAME lookup with matching target (trailing dot)",
			dnsName:        "apps.rosa.example.com",
			expectedTarget: "cluster.example.com.",
			resolverCNAME:  "cluster.example.com.",
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusPass,
			expectedError:  "",
		},
		{
			name:           "CNAME target mismatch",
			dnsName:        "apps.rosa.example.com",
			expectedTarget: "expected.example.com",
			resolverCNAME:  "actual.example.com.",
			resolverErr:    nil,
			expectedStatus: VerifyResultStatusFail,
			expectedError:  "expected expected.example.com, got actual.example.com",
		},
		{
			name:           "DNS lookup failure",
			dnsName:        "nonexistent.example.com",
			expectedTarget: "cluster.example.com",
			resolverCNAME:  "",
			resolverErr:    errors.New("no such host"),
			expectedStatus: VerifyResultStatusFail,
			expectedError:  "no such host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mockResolver := new(MockResolver)
			mockResolver.On("LookupCNAME", mock.Anything, tt.dnsName).Return(tt.resolverCNAME, tt.resolverErr)

			verifier := &DefaultVerifier{
				cfg: DefaultVerifierConfig{
					Timeout:  10 * time.Second,
					Resolver: mockResolver,
				},
			}

			ctx := context.Background()
			var result VerifyResult
			if tt.expectedTarget != "" {
				result = verifier.VerifyCNAMERecord(ctx, tt.dnsName, WithExpectedTarget(tt.expectedTarget))
			} else {
				result = verifier.VerifyCNAMERecord(ctx, tt.dnsName)
			}

			assert.Equal(t, tt.dnsName, result.Name)
			assert.Equal(t, RecordTypeCNAME, result.Type)
			assert.Equal(t, tt.expectedStatus, result.Status)
			assert.Equal(t, tt.expectedTarget, result.ExpectedTarget)
			if tt.resolverCNAME != "" {
				assert.Equal(t, tt.resolverCNAME, result.ActualTarget)
			}
			if tt.expectedError != "" {
				assert.Contains(t, result.ErrorMessage, tt.expectedError)
			}

			mockResolver.AssertExpectations(t)
		})
	}
}

func TestNewDefaultVerifier(t *testing.T) {
	tests := []struct {
		name            string
		opts            []DefaultVerifierOption
		expectedTimeout time.Duration
	}{
		{
			name:            "default configuration",
			opts:            nil,
			expectedTimeout: 10 * time.Second,
		},
		{
			name:            "custom timeout",
			opts:            []DefaultVerifierOption{WithTimeout(5 * time.Second)},
			expectedTimeout: 5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			verifier := NewDefaultVerifier(tt.opts...)

			assert.NotNil(t, verifier)
			assert.Equal(t, tt.expectedTimeout, verifier.cfg.Timeout)
			assert.NotNil(t, verifier.cfg.Resolver)
		})
	}
}

func TestDefaultVerifierConfig_Default(t *testing.T) {
	t.Run("sets default timeout when zero", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultVerifierConfig{}
		cfg.Default()

		assert.Equal(t, 10*time.Second, cfg.Timeout)
		assert.NotNil(t, cfg.Resolver)
	})

	t.Run("preserves existing timeout", func(t *testing.T) {
		t.Parallel()
		cfg := DefaultVerifierConfig{
			Timeout: 5 * time.Second,
		}
		cfg.Default()

		assert.Equal(t, 5*time.Second, cfg.Timeout)
		assert.NotNil(t, cfg.Resolver)
	})

	t.Run("preserves custom resolver", func(t *testing.T) {
		t.Parallel()
		mockResolver := new(MockResolver)
		cfg := DefaultVerifierConfig{
			Resolver: mockResolver,
		}
		cfg.Default()

		assert.Equal(t, mockResolver, cfg.Resolver)
	})
}

func TestWithTimeout(t *testing.T) {
	t.Parallel()
	timeout := WithTimeout(15 * time.Second)
	cfg := &DefaultVerifierConfig{}
	timeout.ConfigureDefaultVerifier(cfg)

	assert.Equal(t, 15*time.Second, cfg.Timeout)
}

func TestWithExpectedTarget(t *testing.T) {
	t.Parallel()
	target := WithExpectedTarget("example.com")
	cfg := &VerifyCNAMERecordConfig{}
	target.ConfigureVerifyCNAMERecord(cfg)

	assert.Equal(t, "example.com", cfg.ExpectedTarget)
}
