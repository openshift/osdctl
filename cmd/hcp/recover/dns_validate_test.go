package recover

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCmdDNSValidate(t *testing.T) {
	cmd := newCmdValidate()

	assert.Equal(t, "validate", cmd.Name())
	assert.Contains(t, cmd.Short, "Validate DNS and networking health")
	assert.Contains(t, cmd.Long, "comprehensive health validation")
}

func TestNewCmdDNSValidate_RequiredFlags(t *testing.T) {
	cmd := newCmdValidate()

	requiredFlags := []string{"cluster-id", "management-cluster", "reason"}
	for _, flag := range requiredFlags {
		f := cmd.Flags().Lookup(flag)
		require.NotNil(t, f, "expected required flag %q to exist", flag)
		ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
		assert.NotEmpty(t, ann, "expected flag %q to be marked required", flag)
	}
}

func TestNewCmdDNSValidate_OptionalFlags(t *testing.T) {
	cmd := newCmdValidate()

	optionalFlags := []struct {
		name         string
		defaultValue string
	}{
		{"debug", "false"},
	}

	for _, tt := range optionalFlags {
		t.Run(tt.name, func(t *testing.T) {
			f := cmd.Flags().Lookup(tt.name)
			require.NotNil(t, f, "expected optional flag %q to exist", tt.name)
			assert.Equal(t, tt.defaultValue, f.DefValue, "unexpected default for flag %q", tt.name)
		})
	}
}

func TestNewCmdDNSValidate_ShorthandFlags(t *testing.T) {
	cmd := newCmdValidate()

	shorthands := map[string]string{
		"cluster-id":         "C",
		"management-cluster": "M",
	}

	for flag, shorthand := range shorthands {
		f := cmd.Flags().Lookup(flag)
		require.NotNil(t, f, "expected flag %q to exist", flag)
		assert.Equal(t, shorthand, f.Shorthand, "unexpected shorthand for flag %q", flag)
	}
}

func TestNewCmdDNSValidate_AcceptsNoArgs(t *testing.T) {
	cmd := newCmdValidate()
	assert.NotNil(t, cmd.Args)
}

func TestDNSValidateOptions_Validate_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		opts    dnsValidateOptions
		wantErr bool
	}{
		{
			name:    "all empty",
			opts:    dnsValidateOptions{},
			wantErr: true,
		},
		{
			name: "missing management-cluster",
			opts: dnsValidateOptions{
				clusterID: "test-cluster",
				reason:    "validation",
			},
			wantErr: true,
		},
		{
			name: "missing reason",
			opts: dnsValidateOptions{
				clusterID:         "test-cluster",
				managementCluster: "dest-mc",
			},
			wantErr: true,
		},
		{
			name: "missing cluster-id",
			opts: dnsValidateOptions{
				managementCluster: "dest-mc",
				reason:            "validation",
			},
			wantErr: true,
		},
		{
			name: "all provided",
			opts: dnsValidateOptions{
				clusterID:         "test-cluster",
				managementCluster: "dest-mc",
				reason:            "validation",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			err := tt.opts.validate(ctx, nil)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidationStatus_Constants(t *testing.T) {
	assert.Equal(t, validationStatus(0), statusPassed)
	assert.Equal(t, validationStatus(1), statusWarning)
	assert.Equal(t, validationStatus(2), statusFailed)
}

func TestPhaseResult_Structure(t *testing.T) {
	result := phaseResult{
		name:    "Test Phase",
		status:  statusPassed,
		details: []string{"detail 1", "detail 2"},
		errors:  []string{"error 1"},
	}

	assert.Equal(t, "Test Phase", result.name)
	assert.Equal(t, statusPassed, result.status)
	assert.Len(t, result.details, 2)
	assert.Len(t, result.errors, 1)
}

func TestValidationResult_Structure(t *testing.T) {
	result := validationResult{
		phases: []phaseResult{
			{name: "Phase 1", status: statusPassed},
			{name: "Phase 2", status: statusWarning},
		},
	}

	assert.Len(t, result.phases, 2)
	assert.Equal(t, "Phase 1", result.phases[0].name)
	assert.Equal(t, "Phase 2", result.phases[1].name)
}

func TestPrintPhaseResult_Passed(t *testing.T) {
	phase := phaseResult{
		name:    "DNS Records",
		status:  statusPassed,
		details: []string{"Found 5 records", "All endpoints present"},
		errors:  []string{},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printPhaseResult(phase)

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "✅")
	assert.Contains(t, output, "DNS Records")
	assert.Contains(t, output, "PASSED")
	assert.Contains(t, output, "Found 5 records")
	assert.Contains(t, output, "All endpoints present")
}

func TestPrintPhaseResult_Warning(t *testing.T) {
	phase := phaseResult{
		name:    "Control Plane",
		status:  statusWarning,
		details: []string{"Some pods not ready"},
		errors:  []string{},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printPhaseResult(phase)

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "⚠️")
	assert.Contains(t, output, "Control Plane")
	assert.Contains(t, output, "WARNING")
	assert.Contains(t, output, "Some pods not ready")
}

func TestPrintPhaseResult_Failed(t *testing.T) {
	phase := phaseResult{
		name:    "Networking",
		status:  statusFailed,
		details: []string{"Checking resources"},
		errors:  []string{"AWSEndpointService not found", "Service missing"},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printPhaseResult(phase)

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "❌")
	assert.Contains(t, output, "Networking")
	assert.Contains(t, output, "FAILED")
	assert.Contains(t, output, "AWSEndpointService not found")
	assert.Contains(t, output, "Service missing")
}

func TestPrintPhaseResult_EmptyLines(t *testing.T) {
	phase := phaseResult{
		name:    "Test Phase",
		status:  statusPassed,
		details: []string{"Line 1", "", "Line 3"},
		errors:  []string{},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printPhaseResult(phase)

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	// Empty lines should be rendered as blank lines
	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 3")
}

func TestPrintValidationSummary_AllPassed(t *testing.T) {
	results := validationResult{
		phases: []phaseResult{
			{name: "Phase 1", status: statusPassed},
			{name: "Phase 2", status: statusPassed},
			{name: "Phase 3", status: statusPassed},
			{name: "Phase 4", status: statusPassed},
		},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printValidationSummary(results, "test-cluster", "test-mc")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "✅ ALL CHECKS PASSED")
	assert.Contains(t, output, "test-cluster")
	assert.Contains(t, output, "test-mc")
	assert.Contains(t, output, "4 passed, 0 warnings, 0 failed")
	assert.Contains(t, output, "DNS recovery validation successful!")
}

func TestPrintValidationSummary_WithWarnings(t *testing.T) {
	results := validationResult{
		phases: []phaseResult{
			{name: "Phase 1", status: statusPassed},
			{name: "Phase 2", status: statusWarning},
			{name: "Phase 3", status: statusWarning},
			{name: "Phase 4", status: statusPassed},
		},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printValidationSummary(results, "test-cluster", "test-mc")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "⚠️  PASSED WITH WARNINGS")
	assert.Contains(t, output, "2 passed, 2 warnings, 0 failed")
	assert.Contains(t, output, "Some warnings detected")
}

func TestPrintValidationSummary_WithFailures(t *testing.T) {
	results := validationResult{
		phases: []phaseResult{
			{name: "Phase 1", status: statusPassed},
			{name: "Phase 2", status: statusFailed},
			{name: "Phase 3", status: statusWarning},
			{name: "Phase 4", status: statusFailed},
		},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printValidationSummary(results, "test-cluster", "test-mc")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "❌ VALIDATION FAILED")
	assert.Contains(t, output, "1 passed, 1 warnings, 2 failed")
	assert.Contains(t, output, "Review failed checks above")
	assert.Contains(t, output, "etcd TLS certificate mismatch")
	assert.Contains(t, output, "KNOWN_ISSUES.md")
}

func TestPrintValidationSummary_CountingLogic(t *testing.T) {
	tests := []struct {
		name         string
		phases       []phaseResult
		wantPassed   int
		wantWarnings int
		wantFailed   int
		wantStatus   string
	}{
		{
			name: "all passed",
			phases: []phaseResult{
				{status: statusPassed},
				{status: statusPassed},
			},
			wantPassed:   2,
			wantWarnings: 0,
			wantFailed:   0,
			wantStatus:   "✅ ALL CHECKS PASSED",
		},
		{
			name: "mixed with warnings",
			phases: []phaseResult{
				{status: statusPassed},
				{status: statusWarning},
				{status: statusPassed},
			},
			wantPassed:   2,
			wantWarnings: 1,
			wantFailed:   0,
			wantStatus:   "⚠️  PASSED WITH WARNINGS",
		},
		{
			name: "mixed with failures",
			phases: []phaseResult{
				{status: statusPassed},
				{status: statusWarning},
				{status: statusFailed},
			},
			wantPassed:   1,
			wantWarnings: 1,
			wantFailed:   1,
			wantStatus:   "❌ VALIDATION FAILED",
		},
		{
			name: "all failed",
			phases: []phaseResult{
				{status: statusFailed},
				{status: statusFailed},
			},
			wantPassed:   0,
			wantWarnings: 0,
			wantFailed:   2,
			wantStatus:   "❌ VALIDATION FAILED",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := validationResult{phases: tt.phases}

			// Capture stderr output
			old := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			printValidationSummary(results, "cluster", "mc")

			w.Close()
			os.Stderr = old

			var buf bytes.Buffer
			_, err := buf.ReadFrom(r)
			require.NoError(t, err)
			output := buf.String()

			assert.Contains(t, output, tt.wantStatus)

			// Verify counts in output
			expectedCounts := fmt.Sprintf("%d passed, %d warnings, %d failed",
				tt.wantPassed, tt.wantWarnings, tt.wantFailed)
			assert.Contains(t, output, expectedCounts)
		})
	}
}

func TestPrintValidationSummary_FormattingAndBoxes(t *testing.T) {
	results := validationResult{
		phases: []phaseResult{
			{name: "Test", status: statusPassed},
		},
	}

	// Capture stderr output
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printValidationSummary(results, "test-cluster-id", "test-management-cluster")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	// Check for box drawing characters
	assert.Contains(t, output, "╔════════════════════════════════════════════════════════════╗")
	assert.Contains(t, output, "║          Validation Summary")
	assert.Contains(t, output, "╚════════════════════════════════════════════════════════════╝")

	// Check for cluster and MC display
	assert.Contains(t, output, "test-cluster-id")
	assert.Contains(t, output, "test-management-cluster")
}

func TestValidationResult_EmptyPhases(t *testing.T) {
	results := validationResult{
		phases: []phaseResult{},
	}

	// Should not panic
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	printValidationSummary(results, "cluster", "mc")

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	require.NoError(t, err)
	output := buf.String()

	assert.Contains(t, output, "0 passed, 0 warnings, 0 failed")
}
