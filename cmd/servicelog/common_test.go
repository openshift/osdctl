package servicelog

import (
	"encoding/json"
	"testing"

	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/stretchr/testify/assert"
)

func TestValidateGoodResponse(t *testing.T) {
	tests := []struct {
		name           string
		clusterMessage servicelog.Message
		goodReply      *servicelog.GoodReply
		expectedError  bool
	}{
		{
			name: "successful_validation",
			clusterMessage: servicelog.Message{
				Severity:    "Info",
				ServiceName: "TestService",
				ClusterUUID: "test-cluster-uuid",
				Summary:     "Test Summary",
				Description: "Test Description",
			},
			goodReply: &servicelog.GoodReply{
				Severity:    "Info",
				ServiceName: "TestService",
				ClusterUUID: "test-cluster-uuid",
				Summary:     "Test Summary",
				Description: "Test Description",
			},
			expectedError: false,
		},
		{
			name:           "invalid_json",
			clusterMessage: servicelog.Message{},
			goodReply:      nil,
			expectedError:  true,
		},
		{
			name: "mismatch_severity",
			clusterMessage: servicelog.Message{
				Severity: "Warning",
			},
			goodReply: &servicelog.GoodReply{
				Severity: "Info",
			},
			expectedError: true,
		},
		{
			name: "mismatch_servicename",
			clusterMessage: servicelog.Message{
				ServiceName: "DifferentService",
			},
			goodReply: &servicelog.GoodReply{
				ServiceName: "TestService",
			},
			expectedError: true,
		},
		{
			name: "mismatch_clusteruuid",
			clusterMessage: servicelog.Message{
				ClusterUUID: "different-cluster-uuid",
			},
			goodReply: &servicelog.GoodReply{
				ClusterUUID: "test-cluster-uuid",
			},
			expectedError: true,
		},
		{
			name: "mismatch_summary",
			clusterMessage: servicelog.Message{
				Summary: "Different Summary",
			},
			goodReply: &servicelog.GoodReply{
				Summary: "Test Summary",
			},
			expectedError: true,
		},
		{
			name: "mismatch_description",
			clusterMessage: servicelog.Message{
				Description: "Different Description",
			},
			goodReply: &servicelog.GoodReply{
				Description: "Test Description",
			},
			expectedError: true,
		},
		{
			name: "empty_body",
			clusterMessage: servicelog.Message{
				Severity: "Error",
			},
			goodReply:     nil,
			expectedError: true,
		},
		{
			name: "valid_body_but_unmatched_severity_field",
			clusterMessage: servicelog.Message{
				Severity: "Critical",
			},
			goodReply: &servicelog.GoodReply{
				Severity: "Error",
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			var err error
			if tt.goodReply != nil {
				body, err = json.Marshal(tt.goodReply)
				assert.NoError(t, err)
			} else {
				body = []byte(`{ invalid json}`)
			}
			validatedReply, err := validateGoodResponse(body, tt.clusterMessage)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, validatedReply)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, validatedReply)
				assert.Equal(t, tt.goodReply, validatedReply)
			}
		})
	}
}

func TestValidateBadResponse(t *testing.T) {
	tests := []struct {
		name          string
		body          []byte
		expectedReply *servicelog.BadReply
		expectedError bool
	}{
		{
			name: "valid_json",
			body: []byte(`{
				"id": "12345",
				"kind": "Error",
				"href": "/service_logs/errors/12345",
				"code": "400",
				"reason": "Bad Request",
				"operation_id": "op-123"
			}`),
			expectedReply: &servicelog.BadReply{
				ID:          "12345",
				Kind:        "Error",
				Href:        "/service_logs/errors/12345",
				Code:        "400",
				Reason:      "Bad Request",
				OperationID: "op-123",
			},
			expectedError: false,
		},
		{
			name:          "invalid_json",
			body:          []byte(`{invalid json}`),
			expectedReply: nil,
			expectedError: true,
		},
		{
			name: "valid_json_but_missing_fields",
			body: []byte(`{
				"id": "12345",
				"kind": "Error",
				"href": "/service_logs/errors/12345"
			}`),
			expectedReply: &servicelog.BadReply{
				ID:   "12345",
				Kind: "Error",
				Href: "/service_logs/errors/12345",
			},
			expectedError: false,
		},
		{
			name: "valid_json_but_with_extra_fields",
			body: []byte(`{
				"id": "12345",
				"kind": "Error",
				"href": "/service_logs/errors/12345",
				"code": "400",
				"reason": "Bad Request",
				"operation_id": "op-123",
				"extra_field": "extra_value"
			}`),
			expectedReply: &servicelog.BadReply{
				ID:          "12345",
				Kind:        "Error",
				Href:        "/service_logs/errors/12345",
				Code:        "400",
				Reason:      "Bad Request",
				OperationID: "op-123",
			},
			expectedError: false,
		},
		{
			name:          "empty_body",
			body:          []byte(``),
			expectedReply: nil,
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validatedReply, err := validateBadResponse(tt.body)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, validatedReply)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, validatedReply)
				assert.Equal(t, tt.expectedReply, validatedReply)
			}
		})
	}
}
