package policies

import (
	"bytes"
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// mockIAMSimulator provides a configurable mock for testing.
type mockIAMSimulator struct {
	// SimulateFunc allows tests to define custom behavior
	SimulateFunc func(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error)
}

func (m *mockIAMSimulator) SimulateCustomPolicy(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error) {
	if m.SimulateFunc != nil {
		return m.SimulateFunc(ctx, params, optFns...)
	}
	return &iam.SimulateCustomPolicyOutput{}, nil
}

// newMockAllowed returns a mock that always returns "allowed"
func newMockAllowed() *mockIAMSimulator {
	return &mockIAMSimulator{
		SimulateFunc: func(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error) {
			return &iam.SimulateCustomPolicyOutput{
				EvaluationResults: []iamtypes.EvaluationResult{
					{
						EvalActionName: aws.String(params.ActionNames[0]),
						EvalDecision:   iamtypes.PolicyEvaluationDecisionTypeAllowed,
					},
				},
			}, nil
		},
	}
}

// newMockDenied returns a mock that always returns "implicitDeny"
func newMockDenied() *mockIAMSimulator {
	return &mockIAMSimulator{
		SimulateFunc: func(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error) {
			return &iam.SimulateCustomPolicyOutput{
				EvaluationResults: []iamtypes.EvaluationResult{
					{
						EvalActionName: aws.String(params.ActionNames[0]),
						EvalDecision:   iamtypes.PolicyEvaluationDecisionTypeImplicitDeny,
					},
				},
			}, nil
		},
	}
}

func testdataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata")
}

func TestDecisionMatchesExpected(t *testing.T) {
	tests := []struct {
		decision string
		expected string
		want     bool
	}{
		{"allowed", "allowed", true},
		{"implicitDeny", "allowed", false},
		{"explicitDeny", "allowed", false},
		{"implicitDeny", "denied", true},
		{"explicitDeny", "denied", true},
		{"allowed", "denied", false},
		{"implicitDeny", "implicitDeny", true},
	}

	for _, tt := range tests {
		t.Run(tt.decision+"_"+tt.expected, func(t *testing.T) {
			got := decisionMatchesExpected(tt.decision, tt.expected)
			if got != tt.want {
				t.Errorf("decisionMatchesExpected(%q, %q) = %v, want %v", tt.decision, tt.expected, got, tt.want)
			}
		})
	}
}

func TestSimulateScenario_Allowed(t *testing.T) {
	mock := newMockAllowed()
	scenario := SimulationScenario{
		Name:      "test allowed",
		Action:    "ec2:DescribeVolumes",
		Resources: []string{"*"},
		Expect:    "allowed",
	}

	result, err := SimulateScenario(context.Background(), mock, "{}", scenario, "test-component")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Pass {
		t.Errorf("expected pass, got fail. Decision: %s", result.Decision)
	}
	if result.Decision != "allowed" {
		t.Errorf("expected decision 'allowed', got %q", result.Decision)
	}
}

func TestSimulateScenario_DeniedWhenExpectedAllowed(t *testing.T) {
	mock := newMockDenied()
	scenario := SimulationScenario{
		Name:      "CFE-1131 standalone CreateTags",
		Action:    "ec2:CreateTags",
		Resources: []string{"arn:aws:ec2:*:*:volume/*"},
		Expect:    "allowed",
	}

	result, err := SimulateScenario(context.Background(), mock, "{}", scenario, "ebs-csi-driver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Pass {
		t.Error("expected fail (denied when allowed expected), got pass")
	}
	if result.Decision != "implicitDeny" {
		t.Errorf("expected decision 'implicitDeny', got %q", result.Decision)
	}
}

func TestSimulateScenario_WithContextKeys(t *testing.T) {
	var capturedInput *iam.SimulateCustomPolicyInput

	mock := &mockIAMSimulator{
		SimulateFunc: func(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error) {
			capturedInput = params
			return &iam.SimulateCustomPolicyOutput{
				EvaluationResults: []iamtypes.EvaluationResult{
					{
						EvalActionName: aws.String(params.ActionNames[0]),
						EvalDecision:   iamtypes.PolicyEvaluationDecisionTypeAllowed,
					},
				},
			}, nil
		},
	}

	scenario := SimulationScenario{
		Name:      "CreateTags during CreateVolume",
		Action:    "ec2:CreateTags",
		Resources: []string{"arn:aws:ec2:*:*:volume/*"},
		Context: map[string]ContextKeyDef{
			"ec2:CreateAction": {
				Type:   "string",
				Values: []string{"CreateVolume"},
			},
		},
		Expect: "allowed",
	}

	_, err := SimulateScenario(context.Background(), mock, "{}", scenario, "ebs-csi-driver")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedInput == nil {
		t.Fatal("SimulateCustomPolicy was not called")
	}

	if len(capturedInput.ContextEntries) != 1 {
		t.Fatalf("expected 1 context entry, got %d", len(capturedInput.ContextEntries))
	}

	entry := capturedInput.ContextEntries[0]
	if *entry.ContextKeyName != "ec2:CreateAction" {
		t.Errorf("expected context key 'ec2:CreateAction', got %q", *entry.ContextKeyName)
	}
	if entry.ContextKeyType != iamtypes.ContextKeyTypeEnumString {
		t.Errorf("expected context type 'string', got %q", entry.ContextKeyType)
	}
	if len(entry.ContextKeyValues) != 1 || entry.ContextKeyValues[0] != "CreateVolume" {
		t.Errorf("expected context values ['CreateVolume'], got %v", entry.ContextKeyValues)
	}
}

func TestSimulateManifest(t *testing.T) {
	mock := newMockAllowed()

	manifest := &SimulationManifest{
		Component:  "test",
		PolicyName: "TestPolicy",
		Scenarios: []SimulationScenario{
			{Name: "test1", Action: "ec2:DescribeVolumes", Resources: []string{"*"}, Expect: "allowed"},
			{Name: "test2", Action: "ec2:DescribeInstances", Resources: []string{"*"}, Expect: "allowed"},
		},
	}

	report, err := SimulateManifest(context.Background(), mock, "{}", manifest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if report.Total != 2 {
		t.Errorf("expected 2 total, got %d", report.Total)
	}
	if report.Passed != 2 {
		t.Errorf("expected 2 passed, got %d", report.Passed)
	}
	if report.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", report.Failed)
	}
}

func TestLoadPolicyDocument(t *testing.T) {
	policyPath := filepath.Join(testdataDir(), "policies", "ROSAAmazonEBSCSIDriverOperatorPolicy.json")
	doc, err := LoadPolicyDocument(policyPath)
	if err != nil {
		t.Fatalf("failed to load policy: %v", err)
	}

	if len(doc) == 0 {
		t.Error("policy document is empty")
	}

	// Verify it contains expected content
	if !bytes.Contains([]byte(doc), []byte("ec2:CreateTags")) {
		t.Error("policy document should contain ec2:CreateTags")
	}
}

func TestLoadSimulationManifest(t *testing.T) {
	manifestPath := filepath.Join(testdataDir(), "manifests", "ebs-csi-driver.yaml")
	manifest, err := LoadSimulationManifest(manifestPath)
	if err != nil {
		t.Fatalf("failed to load manifest: %v", err)
	}

	if manifest.Component != "ebs-csi-driver" {
		t.Errorf("expected component 'ebs-csi-driver', got %q", manifest.Component)
	}
	if manifest.PolicyName != "ROSAAmazonEBSCSIDriverOperatorPolicy" {
		t.Errorf("expected policyName 'ROSAAmazonEBSCSIDriverOperatorPolicy', got %q", manifest.PolicyName)
	}
	if len(manifest.Scenarios) == 0 {
		t.Error("manifest has no scenarios")
	}

	// Check CFE-1131 scenario exists
	found := false
	for _, s := range manifest.Scenarios {
		if s.Action == "ec2:CreateTags" && len(s.Context) == 0 {
			found = true
			if s.Expect != "allowed" {
				t.Errorf("CFE-1131 scenario should expect 'allowed', got %q", s.Expect)
			}
			break
		}
	}
	if !found {
		t.Error("manifest should contain a standalone CreateTags scenario (CFE-1131)")
	}
}

func TestReportFormatTable(t *testing.T) {
	report := &SimulationReport{
		PolicyName: "TestPolicy",
		Total:      2,
		Passed:     1,
		Failed:     1,
		Results: []SimulationResult{
			{Component: "ebs", Action: "ec2:DescribeVolumes", Resource: "*", Decision: "allowed", Expected: "allowed", Pass: true, ContextDesc: "test1"},
			{Component: "ebs", Action: "ec2:CreateTags", Resource: "arn:aws:ec2:*:*:volume/*", Decision: "implicitDeny", Expected: "allowed", Pass: false, ContextDesc: "CFE-1131"},
		},
	}

	var buf bytes.Buffer
	report.FormatTable(&buf)
	output := buf.String()

	if !bytes.Contains([]byte(output), []byte("FAIL")) {
		t.Error("table output should contain FAIL")
	}
	if !bytes.Contains([]byte(output), []byte("PASS")) {
		t.Error("table output should contain PASS")
	}
	if !bytes.Contains([]byte(output), []byte("1/2 passed")) {
		t.Error("table output should contain summary '1/2 passed'")
	}
}

func TestReportFormatJSON(t *testing.T) {
	report := &SimulationReport{
		PolicyName: "TestPolicy",
		Total:      1,
		Passed:     1,
		Results: []SimulationResult{
			{Component: "ebs", Action: "ec2:DescribeVolumes", Decision: "allowed", Expected: "allowed", Pass: true},
		},
	}

	var buf bytes.Buffer
	err := report.FormatJSON(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !bytes.Contains(buf.Bytes(), []byte(`"policyName": "TestPolicy"`)) {
		t.Error("JSON output should contain policyName")
	}
}

func TestReportFormatJUnitXML(t *testing.T) {
	report := &SimulationReport{
		PolicyName: "TestPolicy",
		Total:      2,
		Passed:     1,
		Failed:     1,
		Results: []SimulationResult{
			{Component: "ebs", Action: "ec2:DescribeVolumes", Decision: "allowed", Expected: "allowed", Pass: true, ContextDesc: "test1"},
			{Component: "ebs", Action: "ec2:CreateTags", Resource: "arn:aws:ec2:*:*:volume/*", Decision: "implicitDeny", Expected: "allowed", Pass: false, ContextDesc: "CFE-1131"},
		},
	}

	var buf bytes.Buffer
	err := report.FormatJUnitXML(&buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !bytes.Contains([]byte(output), []byte("<testsuite")) {
		t.Error("JUnit output should contain <testsuite>")
	}
	if !bytes.Contains([]byte(output), []byte("failures=\"1\"")) {
		t.Error("JUnit output should show 1 failure")
	}
	if !bytes.Contains([]byte(output), []byte("PolicyMismatch")) {
		t.Error("JUnit output should contain PolicyMismatch failure type")
	}
}

func TestParseContextKeyType(t *testing.T) {
	tests := []struct {
		input    string
		expected iamtypes.ContextKeyTypeEnum
		wantErr  bool
	}{
		{"string", iamtypes.ContextKeyTypeEnumString, false},
		{"", iamtypes.ContextKeyTypeEnumString, false},
		{"boolean", iamtypes.ContextKeyTypeEnumBoolean, false},
		{"ip", iamtypes.ContextKeyTypeEnumIp, false},
		{"invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseContextKeyType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseContextKeyType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("parseContextKeyType(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
