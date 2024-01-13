package support

import (
	"fmt"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"testing"
)

func Test_setup(t *testing.T) {
	tests := []struct {
		name      string
		post      *Post
		expectErr bool
	}{
		{
			name: "Error - Ends in period",
			post: &Post{
				Problem:    "A problem sentence.",
				Resolution: "A resolution sentence.",
			},
			expectErr: true,
		},
		{
			name: "No error",
			post: &Post{
				Problem:    "A problem sentence",
				Resolution: "A resolution sentence",
			},
			expectErr: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.post.setup()
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", err)
				}
			} else {
				if test.expectErr {
					t.Error("expected err, got nil")
				}
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
			if details := got.Details(); details != fmt.Sprintf("%v. %v", tt.post.Problem, tt.post.Resolution) {
				t.Errorf("buildLimitedSupport() got details = %v, want %v", details, fmt.Sprintf("%v. %v", tt.post.Problem, tt.post.Resolution))
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
