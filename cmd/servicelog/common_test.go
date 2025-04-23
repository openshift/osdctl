package servicelog

import (
	"testing"

	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/stretchr/testify/assert"
)

func TestValidateGoodResponse(t *testing.T) {
	tests := []struct {
		name           string
		clusterMessage servicelog.Message
		goodReply      []byte
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
			goodReply: []byte(`{
				"severity": "Info",
				"service_name": "TestService",
				"cluster_uuid": "test-cluster-uuid",
				"summary": "Test Summary",
				"description": "Test Description"
			}`),
			expectedError: false,
		},
		{
			name: "invalid_json_format",
			clusterMessage: servicelog.Message{
				Severity: "Error",
			},
			goodReply:     []byte(`{ invalid json}`),
			expectedError: true,
		},
		{
			name: "mismatch_severity",
			clusterMessage: servicelog.Message{
				Severity: "Info",
			},
			goodReply: []byte(`{
				"severity": "Warning",
				"service_name": "TestService",
				"cluster_uuid": "test-cluster-uuid",
				"summary": "Test Summary",
				"description": "Test Description"
			}`),
			expectedError: true,
		},
		{
			name: "mismatch_servicename",
			clusterMessage: servicelog.Message{
				ServiceName: "TestService",
			},
			goodReply: []byte(`{
				"severity": "Info",
				"service_name": "DifferentService",
				"cluster_uuid": "test-cluster-uuid",
				"summary": "Test Summary",
				"description": "Test Description"
			}`),
			expectedError: true,
		},
		{
			name: "mismatch_clusteruuid",
			clusterMessage: servicelog.Message{
				ClusterUUID: "test-cluster-uuid",
			},
			goodReply: []byte(`{
				"severity": "Info",
				"service_name": "TestService",
				"cluster_uuid": "different-cluster-uuid",
				"summary": "Test Summary",
				"description": "Test Description"
			}`),
			expectedError: true,
		},
		{
			name: "mismatch_summary",
			clusterMessage: servicelog.Message{
				Summary: "Test Summary",
			},
			goodReply: []byte(`{
				"severity": "Info",
				"service_name": "TestService",
				"cluster_uuid": "test-cluster-uuid",
				"summary": "Different Summary",
				"description": "Test Description"
			}`),
			expectedError: true,
		},
		{
			name: "mismatch_description",
			clusterMessage: servicelog.Message{
				Description: "Test Description",
			},
			goodReply: []byte(`{
				"severity": "Info",
				"service_name": "TestService",
				"cluster_uuid": "test-cluster-uuid",
				"summary": "Test Summary",
				"description": "Different Description"
			}`),
			expectedError: true,
		},
		{
			name: "empty_goodreply",
			clusterMessage: servicelog.Message{
				Severity: "Error",
			},
			goodReply:     []byte(`{}`),
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			validatedReply, err := validateGoodResponse(tt.goodReply, tt.clusterMessage)
			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, validatedReply)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, validatedReply)
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
