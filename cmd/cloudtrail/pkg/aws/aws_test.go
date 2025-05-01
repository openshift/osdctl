package pkg

import (
	"strings"
	"testing"
)

const validEventJSON = `{
	"eventVersion": "1.8",
	"userIdentity": {
		"accountId": "123456789012",
		"sessionContext": {
			"sessionIssuer": {
				"type": "Role",
				"userName": "testUser",
				"arn": "arn:aws:iam::123456789012:role/testRole"
			}
		}
	},
	"awsRegion": "us-east-1",
	"eventID": "abcd-1234",
	"errorCode": "AccessDenied"
}`

func strPtr(s string) *string {
	return &s
}

func TestExtractUserDetails(t *testing.T) {
	tests := []struct {
		testName    string
		input       *string
		expectError bool
		errorSubstr string
	}{
		{
			testName:    "nil_input",
			input:       nil,
			expectError: true,
			errorSubstr: "cannot parse a nil input",
		},
		{
			testName:    "empty_string_input",
			input:       strPtr(""),
			expectError: true,
			// Empty string is treated the same as nil in the logic
			errorSubstr: "cannot parse a nil input",
		},
		{
			testName:    "malformed_json",
			input:       strPtr("{ invalid json "),
			expectError: true,
			errorSubstr: "could not marshal event.CloudTrailEvent",
		},
		{
			testName:    "unsupported_version",
			input:       strPtr(`{"eventVersion": "1.6"}`),
			expectError: true,
			errorSubstr: "unexpected event version",
		},
		{
			testName:    "invalid_version_format",
			input:       strPtr(`{"eventVersion": "abc.def"}`),
			expectError: true,
			errorSubstr: "failed to parse CloudTrail event version",
		},
		{
			testName:    "valid_input_with_supported_version",
			input:       strPtr(validEventJSON),
			expectError: false,
		},
	}

	for _, testCase := range tests {
		// Copy loop variable to avoid closure capture in subtest
		testCase := testCase

		t.Run(testCase.testName, func(t *testing.T) {
			result, err := ExtractUserDetails(testCase.input)

			if testCase.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), testCase.errorSubstr) {
					t.Errorf("expected error to contain %q, got: %v", testCase.errorSubstr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.EventId != "abcd-1234" {
				t.Errorf("expected EventId 'abcd-1234', got: %s", result.EventId)
			}
			if result.UserIdentity.AccountId != "123456789012" {
				t.Errorf("expected AccountId '123456789012', got: %s", result.UserIdentity.AccountId)
			}
			if result.UserIdentity.SessionContext.SessionIssuer.UserName != "testUser" {
				t.Errorf("expected UserName 'testUser', got: %s", result.UserIdentity.SessionContext.SessionIssuer.UserName)
			}
			if result.EventRegion != "us-east-1" {
				t.Errorf("expected EventRegion 'us-east-1', got: %s", result.EventRegion)
			}
			if result.ErrorCode != "AccessDenied" {
				t.Errorf("expected ErrorCode 'AccessDenied', got: %s", result.ErrorCode)
			}
		})
	}
}
