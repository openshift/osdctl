package dns

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

type Verifier interface {
	VerifyARecord(context.Context, string) VerifyResult
	VerifyCNAMERecord(context.Context, string, ...VerifyCNAMERecordOption) VerifyResult
}

type VerifyResult struct {
	Name           string             `json:"name"`
	Type           RecordType         `json:"type"`
	ResolvedIPs    []string           `json:"resolved_ips,omitempty"`    // For A records
	ActualTarget   string             `json:"actual_target,omitempty"`   // For CNAME records (actual)
	ExpectedTarget string             `json:"expected_target,omitempty"` // For CNAME records (expected)
	Status         VerifyResultStatus `json:"status"`                    // "PASS", "FAIL", or "SKIP"
	ErrorMessage   string             `json:"error_message,omitempty"`
	SkipReason     string             `json:"skip_reason,omitempty"` // Explanation for skipped records
}

type VerifyResultStatus string

const (
	VerifyResultStatusPass VerifyResultStatus = "PASS"
	VerifyResultStatusFail VerifyResultStatus = "FAIL"
	VerifyResultStatusSkip VerifyResultStatus = "SKIP"
)

type VerifyCNAMERecordOption interface {
	ConfigureVerifyCNAMERecord(*VerifyCNAMERecordConfig)
}

func NewDefaultVerifier(opts ...DefaultVerifierOption) *DefaultVerifier {
	var cfg DefaultVerifierConfig

	cfg.Option(opts...)
	cfg.Default()

	return &DefaultVerifier{
		cfg: cfg,
	}
}

type DefaultVerifier struct {
	cfg DefaultVerifierConfig
}

// VerifyARecord tests an A record and returns the result
func (v *DefaultVerifier) VerifyARecord(ctx context.Context, path string) VerifyResult {
	url, err := url.Parse(path)
	if err != nil {
		return VerifyResult{
			Name:         path,
			Type:         RecordTypeA,
			Status:       VerifyResultStatusFail,
			ErrorMessage: fmt.Sprintf("invalid URL: %v", err),
		}
	}

	host := url.Hostname()
	if host == "" {
		host = path
	}

	rec := VerifyResult{
		Name: host,
		Type: RecordTypeA,
	}

	ips, err := v.cfg.Resolver.LookupHost(ctx, host)

	if err != nil {
		rec.Status = VerifyResultStatusFail
		rec.ErrorMessage = err.Error()
	} else {
		rec.ResolvedIPs = ips
		rec.Status = VerifyResultStatusPass
	}

	return rec
}

// VerifyCNAMERecord tests a CNAME record and validates it points to the expected target
func (v *DefaultVerifier) VerifyCNAMERecord(ctx context.Context, dnsName string, opts ...VerifyCNAMERecordOption) VerifyResult {
	var cfg VerifyCNAMERecordConfig
	cfg.Option(opts...)

	rec := VerifyResult{
		Name: dnsName,
		Type: RecordTypeCNAME,
	}
	if cfg.ExpectedTarget != "" {
		rec.ExpectedTarget = cfg.ExpectedTarget
	}

	cname, err := v.cfg.Resolver.LookupCNAME(ctx, dnsName)
	if err != nil {
		rec.Status = VerifyResultStatusFail
		rec.ErrorMessage = err.Error()

		return rec
	}
	rec.ActualTarget = cname

	if cfg.ExpectedTarget == "" {
		rec.Status = "PASS"
		return rec
	}

	// Validate CNAME target matches expected value
	// DNS CNAME targets often include trailing dot, so normalize both
	normalizedCNAME := strings.TrimSuffix(cname, ".")
	normalizedExpected := strings.TrimSuffix(cfg.ExpectedTarget, ".")
	if normalizedCNAME == normalizedExpected {
		rec.Status = VerifyResultStatusPass
	} else {
		rec.Status = VerifyResultStatusFail
		rec.ErrorMessage = fmt.Sprintf("expected %s, got %s", normalizedExpected, normalizedCNAME)
	}

	return rec
}

type VerifyCNAMERecordConfig struct {
	ExpectedTarget string
}

func (c *VerifyCNAMERecordConfig) Option(opts ...VerifyCNAMERecordOption) {
	for _, opt := range opts {
		opt.ConfigureVerifyCNAMERecord(c)
	}
}

type Resolver interface {
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupCNAME(ctx context.Context, name string) (string, error)
}

type DefaultVerifierConfig struct {
	Timeout  time.Duration
	Resolver Resolver
}

func (c *DefaultVerifierConfig) Option(opts ...DefaultVerifierOption) {
	for _, opt := range opts {
		opt.ConfigureDefaultVerifier(c)
	}
}

func (c *DefaultVerifierConfig) Default() {
	if c.Timeout == 0 {
		c.Timeout = 10 * time.Second
	}

	if c.Resolver == nil {
		c.Resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: c.Timeout,
				}
				return d.DialContext(ctx, network, address)
			},
		}
	}
}

type DefaultVerifierOption interface {
	ConfigureDefaultVerifier(*DefaultVerifierConfig)
}
