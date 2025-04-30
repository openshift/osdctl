package pkg

import (
	"strings"
	"testing"
)

func str_ptr(s string) *string {
	return &s
}

func TestExtractUserDetails(t *testing.T) {
	tests := []struct {
		test_name    string
		input        *string
		expect_error bool
		error_substr string
	}{
		{
			test_name:    "nil_input",
			input:        nil,
			expect_error: true,
			error_substr: "cannot parse a nil input",
		},
		{
			test_name:    "empty_input",
			input:        str_ptr(""),
			expect_error: true,
			error_substr: "cannot parse a nil input",
		},
		{
			test_name:    "malformed_json",
			input:        str_ptr("{ invalid json "),
			expect_error: true,
			error_substr: "could not marshal event.CloudTrailEvent",
		},
		{
			test_name:    "unsupported_version",
			input:        str_ptr(`{"eventVersion": "1.6"}`),
			expect_error: true,
			error_substr: "unexpected event version",
		},
		{
			test_name:    "invalid_version_format",
			input:        str_ptr(`{"eventVersion": "abc.def"}`),
			expect_error: true,
			error_substr: "failed to parse CloudTrail event version",
		},
		{
			test_name:    "valid_input_with_supported_version",
			input:        str_ptr(valid_event_json),
			expect_error: false,
		},
	}

	for _, test_case := range tests {
		test_case := test_case
		t.Run(test_case.test_name, func(t *testing.T) {
			result, err := ExtractUserDetails(test_case.input)

			if test_case.expect_error {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), test_case.error_substr) {
					t.Errorf("expected error to contain %q, got: %v", test_case.error_substr, err)
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

const valid_event_json = `{
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
