package jumphost

import (
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTagFilters(t *testing.T) {
	tests := []struct {
		name          string
		tags          []types.Tag
		expected      []types.Filter
		expectedPanic bool
	}{
		{
			name:     "empty_input",
			tags:     nil,
			expected: nil,
		},
		{
			name: "single_tag",
			tags: []types.Tag{{Key: aws.String("Environment"), Value: aws.String("Production")}},
			expected: []types.Filter{
				{
					Name:   aws.String("tag:Environment"),
					Values: []string{"Production"},
				},
			},
		},
		{
			name: "multiple_tags",
			tags: []types.Tag{
				{Key: aws.String("App"), Value: aws.String("Backend")},
				{Key: aws.String("Team"), Value: aws.String("DevOps")},
			},
			expected: []types.Filter{
				{Name: aws.String("tag:App"), Values: []string{"Backend"}},
				{Name: aws.String("tag:Team"), Values: []string{"DevOps"}},
			},
		},
		{
			name: "tags_key_nil_val_nil",
			tags: []types.Tag{
				{Key: nil, Value: nil},
			},
			expectedPanic: true,
		},
		{
			name: "tag_with_nil_key",
			tags: []types.Tag{
				{Key: nil, Value: aws.String("Production")},
			},
			expectedPanic: true,
		},
		{
			name: "tag_with_nil_value",
			tags: []types.Tag{
				{Key: aws.String("Environment"), Value: nil},
			},
			expectedPanic: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					if !tt.expectedPanic {
					}
				}
			}()
			result := generateTagFilters(tt.tags)
			if !tt.expectedPanic {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestValidateCluster(t *testing.T) {
	tests := []struct {
		name     string
		cluster  *cmv1.Cluster
		errorMsg string
	}{
		{
			name:     "nil_cluster",
			cluster:  nil,
			errorMsg: "unexpected error, nil cluster provided",
		},
		{
			name: "valid_aws_cluster",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).AWS(cmv1.NewAWS()).Build()
				return cluster1
			}(),
		},
		{
			name: "gcp_cluster",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp")).GCP(cmv1.NewGCP()).Build()
				return cluster1
			}(),
			errorMsg: "only supports aws, got gcp",
		},
		{
			name: "gcp_non_sts_cluster",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp")).GCP(cmv1.NewGCP()).Build()
				return cluster1
			}(),
			errorMsg: "only supports aws, got gcp",
		},

		{
			name: "invalid_cluster",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("non-aws")).Build()
				return cluster1
			}(),
			errorMsg: "only supports aws, got non-aws",
		},
		{
			name: "aws_sts_cluster",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("aws")).AWS(cmv1.NewAWS().STS(cmv1.NewSTS().ExternalID("sts"))).Build()
				return cluster1
			}(),
			errorMsg: "only supports non-STS clusters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCluster(tt.cluster)
			if tt.errorMsg != "" && err != nil {
				assert.Error(t, err)
				assert.Equal(t, err, fmt.Errorf("%s", tt.errorMsg))
				return
			}
			assert.NoError(t, err)
		})
	}
}
