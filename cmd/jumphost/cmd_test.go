package jumphost

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/stretchr/testify/assert"
)

func TestGenerateTagFilters(t *testing.T) {
	tests := []struct {
		name     string
		tags     []types.Tag
		expected []types.Filter
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := generateTagFilters(tt.tags)
			assert.Equal(t, tt.expected, filters)
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
			name: "non_aws_cloud_provider",
			cluster: func() *cmv1.Cluster {
				cluster1, _ := cmv1.NewCluster().CloudProvider(cmv1.NewCloudProvider().ID("gcp")).AWS(cmv1.NewAWS()).Build()
				return cluster1
			}(),
			errorMsg: "only supports aws, got gcp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCluster(tt.cluster)
			if err != nil {
				assert.Error(t, err, tt.errorMsg)
				return
			}
			assert.NoError(t, err)
		})
	}
}
