package support

import (
	"net/http"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func TestCreateDeleteRequest(t *testing.T) {
	conn, _ := sdk.NewConnectionBuilder().URL("https://api.fake.com").Build()

	cluster, _ := cmv1.NewCluster().ID("cluster-123").Build()

	tests := []struct {
		name         string
		cluster      *cmv1.Cluster
		reasonID     string
		expectErr    bool
		expectedPath string
	}{
		{
			name:         "valid_cluster_and_reason_ID",
			cluster:      cluster,
			reasonID:     "reason-456",
			expectErr:    false,
			expectedPath: "/api/clusters_mgmt/v1/clusters/cluster-123/limited_support_reasons/reason-456",
		},
		{
			name:         "empty_reason_ID",
			cluster:      cluster,
			reasonID:     "",
			expectErr:    false,
			expectedPath: "/api/clusters_mgmt/v1/clusters/cluster-123/limited_support_reasons/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := createDeleteRequest(conn, tt.cluster, tt.reasonID)

			if tt.expectErr {
				assert.Error(t, err)
				assert.Nil(t, request)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, request)
			}
		})
	}
}

func TestCheckDelete(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      []byte
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "success_no_content",
			status:  http.StatusNoContent,
			body:    []byte(""),
			wantErr: false,
		},
		{
			name:      "invalid_JSON",
			status:    http.StatusBadRequest,
			body:      []byte("invalid-json"),
			wantErr:   true,
			errSubstr: "server returned invalid JSON",
		},
		{
			name:    "valid_JSON_and_can_unmarshal",
			status:  http.StatusBadRequest,
			body:    []byte(`{"message": "something went wrong"}`),
			wantErr: false, // because checkDelete doesn't return error on parsed bad reply
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := createMockResponse(tt.status, tt.body, nil)
			err := checkDelete(resp)

			if (err != nil) != tt.wantErr {
				t.Errorf("checkDelete() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("expected error to contain %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

// createMockResponse uses reflect and unsafe to inject fields into sdk.Response
func createMockResponse(status int, body []byte, header http.Header) *sdk.Response {
	resp := new(sdk.Response)
	val := reflect.ValueOf(resp).Elem()

	setUnexportedField := func(fieldName string, value interface{}) {
		field := val.FieldByName(fieldName)
		ptr := reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
		ptr.Set(reflect.ValueOf(value))
	}

	setUnexportedField("status", status)
	if header == nil {
		header = http.Header{}
	}
	setUnexportedField("header", header)
	setUnexportedField("body", body)

	return resp
}
