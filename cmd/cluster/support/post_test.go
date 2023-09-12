package support

import (
	"fmt"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"testing"
)

func Test_buildLimitedSupport(t *testing.T) {
	type args struct {
		misconfiguration string
		problem          string
		resolution       string
	}
	tests := []struct {
		name        string
		args        args
		wantSummary string
	}{
		{
			name: "Builds a limited support struct for cloud misconfiguration",
			args: args{
				misconfiguration: "cloud",
				problem:          "test problem cloud",
				resolution:       "test resolution cloud",
			},
			wantSummary: LimitedSupportSummaryCloud,
		},
		{
			name: "Builds a limited support struct for cluster misconfiguration",
			args: args{
				misconfiguration: "cluster",
				problem:          "test problem cluster",
				resolution:       "test resolution cluster",
			},
			wantSummary: LimitedSupportSummaryCluster,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildLimitedSupport(tt.args.misconfiguration, tt.args.problem, tt.args.resolution)
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
			if details := got.Details(); details != fmt.Sprintf("%v. %v", tt.args.problem, tt.args.resolution) {
				t.Errorf("buildLimitedSupport() got details = %v, want %v", details, fmt.Sprintf("%v. %v", tt.args.problem, tt.args.resolution))
			}
		})
	}
}

func Test_buildInternalServiceLog(t *testing.T) {
	type args struct {
		externalId       string
		internalId       string
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
				externalId:       "abc-123",
				internalId:       "def456",
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "subid123",
			},
		},
		{
			name: "Builds a log entry struct with subscription ID",
			args: args{
				externalId:       "abc-123",
				internalId:       "def456",
				limitedSupportId: "test-ls-id",
				evidence:         "this is evidence",
				subscriptionId:   "",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildInternalServiceLog(tt.args.externalId, tt.args.internalId, tt.args.limitedSupportId, tt.args.evidence, tt.args.subscriptionId)
			if err != nil {
				t.Errorf("buildInternalServiceLog() error = %v, wantErr %v", err, false)
				return
			}
			if clusterUUID := got.ClusterUUID(); clusterUUID != tt.args.externalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterUUID, tt.args.externalId)
			}

			if clusterID := got.ClusterID(); clusterID != tt.args.internalId {
				t.Errorf("buildInternalServiceLog() got clusterUUID = %v, want %v", clusterID, tt.args.internalId)
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
