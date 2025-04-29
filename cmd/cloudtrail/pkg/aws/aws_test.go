package pkg

import (
	"strings"
	"testing"
)

func strPtr(s string) *string {
	return &s
}

func TestExtractUserDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       *string
		expectError bool
		errorSubstr string
	}{
		{
			name:        "nil input",
			input:       nil,
			expectError: true,
			errorSubstr: "cannot parse a nil input",
		},
		{
			name:        "empty input",
			input:       strPtr(""),
			expectError: true,
			errorSubstr: "cannot parse a nil input",
		},
		{
			name:        "malformed JSON",
			input:       strPtr("{ invalid json "),
			expectError: true,
			errorSubstr: "could not marshal event.CloudTrailEvent",
		},
		{
			name:        "unsupported version",
			input:       strPtr(`{"eventVersion": "1.6"}`),
			expectError: true,
			errorSubstr: "unexpected event version",
		},
		{
			name:        "invalid version format",
			input:       strPtr(`{"eventVersion": "abc.def"}`),
			expectError: true,
			errorSubstr: "failed to parse CloudTrail event version",
		},
		{
			name:        "valid input with supported version",
			input:       strPtr(validEventJSON),
			expectError: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ExtractUserDetails(tt.input)

			if tt.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errorSubstr) {
					t.Errorf("expected error to contain %q, got: %v", tt.errorSubstr, err)
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
