package cluster

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	capav1beta2 "sigs.k8s.io/cluster-api-provider-aws/v2/api/v1beta2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockCleanupAWSClient struct {
	describeInstancesResp  *ec2.DescribeInstancesOutput
	terminateInstancesResp *ec2.TerminateInstancesOutput
}

func (m mockCleanupAWSClient) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	return m.describeInstancesResp, nil
}

func (m mockCleanupAWSClient) TerminateInstances(ctx context.Context, params *ec2.TerminateInstancesInput, optFns ...func(options *ec2.Options)) (*ec2.TerminateInstancesOutput, error) {
	return m.terminateInstancesResp, nil
}

// newTestCluster assembles a *cmv1.Cluster while handling the error to help out with inline test-case generation
func newTestCluster(t *testing.T, cb *cmv1.ClusterBuilder) *cmv1.Cluster {
	cluster, err := cb.Build()
	if err != nil {
		t.Fatalf("failed to build cluster: %s", err)
	}

	return cluster
}

func Test_cleanup_RemediateOCPBUGS23174(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := capav1beta2.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		c         *cleanup
		expectErr bool
	}{
		{
			name: "awsmachines match EC2 instances",
			c: &cleanup{
				awsClient: mockCleanupAWSClient{
					describeInstancesResp: &ec2.DescribeInstancesOutput{
						Reservations: []types.Reservation{
							{
								Instances: []types.Instance{
									{
										InstanceId: aws.String("i-0123456789"),
									},
								},
							},
						},
					},
				},
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(&capav1beta2.AWSMachine{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"cluster.x-k8s.io/cluster-name": "0123456789",
						},
					},
					Spec: capav1beta2.AWSMachineSpec{InstanceID: aws.String("i-0123456789")},
				}).Build(),
				cluster: newTestCluster(t, cmv1.NewCluster().ID("0123456789")),
			},
		},
		{
			name: "leaked EC2 instances",
			c: &cleanup{
				awsClient: mockCleanupAWSClient{
					describeInstancesResp: &ec2.DescribeInstancesOutput{
						Reservations: []types.Reservation{
							{
								Instances: []types.Instance{
									{
										InstanceId: aws.String("i-0123456789"),
									},
								},
							},
						},
					},
				},
				client:  fake.NewClientBuilder().WithScheme(scheme).Build(),
				cluster: newTestCluster(t, cmv1.NewCluster().ID("0123456789")),
				Yes:     true,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.c.RemediateOCPBUGS23174(context.Background())
			if err != nil {
				if !test.expectErr {
					t.Errorf("expected no err, got %v", err)
				}
			}

			if test.expectErr {
				t.Errorf("expected err, got nil")
			}
		})
	}
}
