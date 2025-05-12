package support

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"testing"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/stretchr/testify/assert"
)

func TestValidateResolutionString(t *testing.T) {
	tests := []struct {
		input         string
		errorExpected bool
	}{
		{"resolution.", true},
		{"no-dot-at-end", false},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			err := validateResolutionString(test.input)
			if (err == nil) == test.errorExpected {
				t.Errorf("For input '%s', expected an error: %t, but got: %v", test.input, test.errorExpected, err)
			}
		})
	}
}

func Test_buildLimitedSupport(t *testing.T) {
	tests := []struct {
		name        string
		post        *Post
		wantSummary string
	}{
		{
			name: "Builds a limited support struct for cloud misconfiguration",
			post: &Post{
				Misconfiguration: cloud,
				Problem:          "test problem cloud",
				Resolution:       "test resolution cloud",
			},
			wantSummary: LimitedSupportSummaryCloud,
		},
		{
			name: "Builds a limited support struct for cluster misconfiguration",
			post: &Post{
				Misconfiguration: cluster,
				Problem:          "test problem cluster",
				Resolution:       "test resolution cluster",
			},
			wantSummary: LimitedSupportSummaryCluster,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.post.buildLimitedSupport()
			if err != nil {
				t.Errorf("buildLimitedSupport() error = %v, wantErr %v", err, false)
				return
			}
			if summary := got.Summary(); summary != tt.wantSummary {
				t.Errorf("buildLimitedSupport() got summary = %v, want %v", summary, tt.wantSummary)
			}
			if detectionType := got.DetectionType(); detectionType != cmv1.DetectionTypeManual {
				t.Errorf("buildLimitedSupport() got detectionType = %v, want %v", detectionType, cmv1.DetectionTypeManual)
			}
			if details := got.Details(); details != fmt.Sprintf("%s %s", tt.post.Problem, tt.post.Resolution) {
				t.Errorf("buildLimitedSupport() got details = %s, want %s", details, fmt.Sprintf("%s %s", tt.post.Problem, tt.post.Resolution))
			}
		})
	}
}

func Test_buildInternalServiceLog(t *testing.T) {
	const (
		externalId = "abc-123"
		internalId = "def456"
	)

	type args struct {
		limitedSupportId string
		evidence         string
		subscriptionId   string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "Builds a log entry struct with subscription ID",
			args: args{
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "subid123",
			},
		},
		{
			name: "Builds a log entry struct without subscription ID",
			args: args{
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster, err := cmv1.NewCluster().ExternalID(externalId).ID(internalId).Build()
			if err != nil {
				t.Error(err)
			}

			p := &Post{cluster: cluster, Evidence: tt.args.evidence}

			got, err := p.buildInternalServiceLog(tt.args.limitedSupportId, tt.args.subscriptionId)
			if err != nil {
				t.Errorf("buildInternalServiceLog() error = %v, wantErr %v", err, false)
				return
			}
			if clusterUUID := got.ClusterUUID(); clusterUUID != externalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterUUID, externalId)
			}

			if clusterID := got.ClusterID(); clusterID != internalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterID, internalId)
			}

			if internalOnly := got.InternalOnly(); internalOnly != true {
				t.Errorf("buildInternalServiceLog() got internalOnly = %v, want %v", internalOnly, true)
			}

			if severity := got.Severity(); severity != InternalServiceLogSeverity {
				t.Errorf("buildInternalServiceLog() got severity = %v, want %v", severity, InternalServiceLogSeverity)
			}

			if serviceName := got.ServiceName(); serviceName != InternalServiceLogServiceName {
				t.Errorf("buildInternalServiceLog() got serviceName = %v, want %v", serviceName, InternalServiceLogServiceName)
			}

			if summary := got.Summary(); summary != InternalServiceLogSummary {
				t.Errorf("buildInternalServiceLog() got summary = %v, want %v", summary, InternalServiceLogSummary)
			}

			if description := got.Description(); description != fmt.Sprintf("%v - %v", tt.args.limitedSupportId, tt.args.evidence) {
				t.Errorf("buildInternalServiceLog() got description = %v, want %v", description, fmt.Sprintf("%v - %v", tt.args.limitedSupportId, tt.args.evidence))
			}

			if subscriptionID := got.SubscriptionID(); subscriptionID != tt.args.subscriptionId {
				t.Errorf("buildInternalServiceLog() got subscriptionID = %v, want %v", subscriptionID, tt.args.subscriptionId)
			}
		})
	}
}

func TestPostCheck(t *testing.T) {
	tests := []struct {
		name           string
		post           Post
		expectError    bool
		errorSubstring string
	}{
		{
			name: "Valid_template_usage_no_conflicting_flags",
			post: Post{
				Template: "some-template",
			},
			expectError: false,
		},
		{
			name: "Template_with_other_fields_set_should_error",
			post: Post{
				Template:         "some-template",
				Problem:          "issue",
				Resolution:       "fix it",
				Misconfiguration: "something wrong",
				Evidence:         "log.txt",
			},
			expectError:    true,
			errorSubstring: "--template flag is used",
		},
		{
			name: "No_template_resolution_ends_with_'.'_should_error",
			post: Post{
				Problem:          "something broke",
				Resolution:       "just reboot.",
				Misconfiguration: "bad config",
			},
			expectError:    true,
			errorSubstring: "should not end with a `.`",
		},
		{
			name: "No_template_resolution_ends_with_'?'_should_error",
			post: Post{
				Problem:          "why does it crash",
				Resolution:       "have you tried turning it off?",
				Misconfiguration: "unknown",
			},
			expectError:    true,
			errorSubstring: "--resolution should not end in punctuation",
		},
		{
			name: "Valid_no_template_post",
			post: Post{
				Problem:          "App crashes on boot",
				Resolution:       "Reinstall the app",
				Misconfiguration: "corrupt binary",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.post.check()
			if tt.expectError {
				assert.Error(t, err)
				if tt.errorSubstring != "" {
					assert.Contains(t, err.Error(), tt.errorSubstring)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestParseUserParameters(t *testing.T) {
	tests := []struct {
		name           string
		templateParams []string
		expectFatal    bool
		expectedNames  []string
		expectedValues []string
	}{
		{
			name:           "Valid_parameters",
			templateParams: []string{"FOO=bar", "KEY=value"},
			expectFatal:    false,
			expectedNames:  []string{"${FOO}", "${KEY}"},
			expectedValues: []string{"bar", "value"},
		},
		{
			name:           "Missing_equals_sign",
			templateParams: []string{"FOOBAR"},
			expectFatal:    true,
		},
		{
			name:           "Empty_key",
			templateParams: []string{"=value"},
			expectFatal:    true,
		},
		{
			name:           "Empty_value",
			templateParams: []string{"FOO="},
			expectFatal:    true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectFatal {
				if !runsAndFailsWithFatal(tt.templateParams) {
					t.Errorf("expected fatal error but did not get one")
				}
			} else {
				userParameterNames = []string{}
				userParameterValues = []string{}

				post := &Post{TemplateParams: tt.templateParams}
				post.parseUserParameters()

				assert.ElementsMatch(t, userParameterNames, tt.expectedNames)
				assert.ElementsMatch(t, userParameterValues, tt.expectedValues)
			}
		})
	}
}

func runsAndFailsWithFatal(params []string) bool {
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessFatal")
	cmd.Env = append(os.Environ(), "GO_WANT_HELPER_PROCESS=1", "TEMPLATE_PARAMS="+strings.Join(params, ","))
	err := cmd.Run()
	return err != nil && !cmd.ProcessState.Success()
}

func TestHelperProcessFatal(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	params := strings.Split(os.Getenv("TEMPLATE_PARAMS"), ",")
	post := &Post{TemplateParams: params}
	post.parseUserParameters()
}

func TestReplaceFlags(t *testing.T) {
	tests := []struct {
		name         string
		template     *TemplateFile
		flagName     string
		flagValue    string
		expectFatal  bool
		expectedText string
	}{
		{
			name: "Flag_exists_and_replaced",
			template: &TemplateFile{
				Details: "This is a template with the FOO flag.",
			},
			flagName:     "FOO",
			flagValue:    "FOOBAR",
			expectFatal:  false,
			expectedText: "This is a template with the FOOBAR flag.",
		},
		{
			name: "Flag_does_not_exist_in_template_should_error",
			template: &TemplateFile{
				Details: "This is a template without the FLAG flag.",
			},
			flagName:     "FOO",
			flagValue:    "FOOBAR",
			expectFatal:  true,
			expectedText: "This is a template without the FLAG flag.",
		},
		{
			name: "Flag_value_is_empt__should_error",
			template: &TemplateFile{
				Details: "This is a template with the FOO flag.",
			},
			flagName:     "FOO",
			flagValue:    "",
			expectFatal:  true,
			expectedText: "This is a template with the FOO flag.",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// Use the helper function to test fatal log behavior
			if tt.expectFatal {
				params := []string{tt.flagName, tt.flagValue}
				if runsAndFailsWithFatal(params) {

					return
				}
				t.Errorf("Expected fatal log, but none occurred")
			} else {

				p := &Post{}
				p.replaceFlags(tt.template, tt.flagName, tt.flagValue)
				assert.Equal(t, tt.expectedText, tt.template.Details)
			}
		})
	}
}

func TestPostFindLeftovers(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "No_leftovers",
			input:    "This is a template with no parameters.",
			expected: []string{},
		},
		{
			name:     "Single_leftover",
			input:    "This is a template with ${PARAM1}.",
			expected: []string{"${PARAM1}"},
		},
		{
			name:     "Multiple_leftovers",
			input:    "This is a template with ${PARAM1} and ${PARAM2}.",
			expected: []string{"${PARAM1}", "${PARAM2}"},
		},
		{
			name:     "Nested_leftovers",
			input:    "This is a template with ${PARAM1} and ${NESTED_PARAM}.",
			expected: []string{"${PARAM1}", "${NESTED_PARAM}"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			p := &Post{}
			actual := p.findLeftovers(tt.input)
			assert.ElementsMatch(t, tt.expected, actual)
		})
	}
}

func TestPostCheckLeftovers(t *testing.T) {
	tests := []struct {
		name               string
		templateDetails    string
		expectedFatalError bool
		expectedLog        string
	}{
		{
			name:               "No_leftovers",
			templateDetails:    "This is a template with no parameters.",
			expectedFatalError: false,
			expectedLog:        "",
		},
		{
			name:               "Single_leftover_missing_parameter",
			templateDetails:    "This is a template with ${PARAM1}.",
			expectedFatalError: true,
			expectedLog:        "The one of the template files is using '${PARAM1}' parameter, but '--param' flag is not set for this one. Use '-p PARAM1=\"FOOBAR\"' to fix this.",
		},
		{
			name:               "Multiple_leftover__missing_parameters",
			templateDetails:    "This is a template with ${PARAM1} and ${PARAM2}.",
			expectedFatalError: true,
			expectedLog:        "The one of the template files is using '${PARAM1}' parameter, but '--param' flag is not set for this one. Use '-p PARAM1=\"FOOBAR\"' to fix this.",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectedFatalError {

				ok := runsAndFailsWithFatal([]string{tt.templateDetails})
				assert.True(t, ok)
				return
			}

			var capturedLogs strings.Builder
			log.SetOutput(&capturedLogs)
			defer log.SetOutput(nil)

			template := &TemplateFile{
				Details: tt.templateDetails,
			}
			p := &Post{}
			p.checkLeftovers(template)

			assert.Contains(t, capturedLogs.String(), tt.expectedLog)
		})
	}
}

func TestPrintLimitedSupportReason(t *testing.T) {
	limitedSupportReason1, _ := cmv1.NewLimitedSupportReason().
		ID("ls-123").
		Summary("Summary for limited support").
		Details("Details for limited support").
		Build()

	limitedSupportReasonEmpty, _ := cmv1.NewLimitedSupportReason().Build()

	tests := []struct {
		name             string
		reason           *cmv1.LimitedSupportReason
		expectError      bool
		expectedContains string
	}{
		{
			name:             "valid_reason_prints_successfully",
			reason:           limitedSupportReason1,
			expectError:      false,
			expectedContains: "Summary for limited support",
		},
		{
			name:             "empty_reason_object",
			reason:           limitedSupportReasonEmpty,
			expectError:      false,
			expectedContains: "kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			originalStdout := os.Stdout

			reader, writer, err := os.Pipe()
			assert.NoError(t, err)

			os.Stdout = writer

			err = printLimitedSupportReason(tt.reason)

			writer.Close()
			os.Stdout = originalStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, reader)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, buf.String(), tt.expectedContains)
			}
		})
	}
}

func TestPrintInternalServiceLog(t *testing.T) {
	logEntryValid, _ := slv1.NewLogEntry().
		ID("log-001").
		Summary("Summary of internal service log").
		Description("Description of the log").
		Build()

	logEntryEmpty, _ := slv1.NewLogEntry().Build()

	tests := []struct {
		name             string
		logEntry         *slv1.LogEntry
		expectError      bool
		expectedContains string
	}{
		{
			name:             "valid_log_entry_prints_successfully",
			logEntry:         logEntryValid,
			expectError:      false,
			expectedContains: "Summary of internal service log",
		},
		{
			name:             "empty_log_entry_still_prints_'kind'",
			logEntry:         logEntryEmpty,
			expectError:      false,
			expectedContains: "kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			old := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := printInternalServiceLog(tt.logEntry)

			w.Close()
			os.Stdout = old

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)

			output := buf.String()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, output, tt.expectedContains)
			}
		})
	}
}
