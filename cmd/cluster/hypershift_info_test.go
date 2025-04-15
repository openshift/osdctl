package cluster

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	aws1 "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/provider/aws"
)

type MockAWSClient struct {
	aws.Client
	mock.Mock
}

func (m *MockAWSClient) ListHostedZones(input *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*route53.ListHostedZonesOutput), args.Error(1)
}

func (m *MockAWSClient) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (*route53.ListResourceRecordSetsOutput, error) {
	args := m.Called(input)
	if args.Get(0) != nil {
		return args.Get(0).(*route53.ListResourceRecordSetsOutput), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockAWSClient) DescribeVpcEndpointConnections(input *ec2.DescribeVpcEndpointConnectionsInput) (*ec2.DescribeVpcEndpointConnectionsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*ec2.DescribeVpcEndpointConnectionsOutput), args.Error(1)
}

func (m *MockAWSClient) DescribeVpcEndpoints(input *ec2.DescribeVpcEndpointsInput) (*ec2.DescribeVpcEndpointsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*ec2.DescribeVpcEndpointsOutput), args.Error(1)
}

func (m *MockAWSClient) DescribeVpcEndpointServices(input *ec2.DescribeVpcEndpointServicesInput) (*ec2.DescribeVpcEndpointServicesOutput, error) {
	args := m.Called(input)
	if args.Get(0) != nil {
		return args.Get(0).(*ec2.DescribeVpcEndpointServicesOutput), args.Error(1)
	}
	return nil, args.Error(1)

}

func (m *MockAWSClient) DescribeV2LoadBalancers(input *elasticloadbalancingv2.DescribeLoadBalancersInput) (*elasticloadbalancingv2.DescribeLoadBalancersOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*elasticloadbalancingv2.DescribeLoadBalancersOutput), args.Error(1)
}

func (m *MockAWSClient) DescribeV2Tags(input *elasticloadbalancingv2.DescribeTagsInput) (*elasticloadbalancingv2.DescribeTagsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*elasticloadbalancingv2.DescribeTagsOutput), args.Error(1)
}

func (m *MockAWSClient) GetVpcEndpointServices(clusterID string) ([]string, error) {
	args := m.Called(clusterID)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockAWSClient) GetVpcEndpointConnections(services []string) ([]string, error) {
	args := m.Called(services)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockAWSClient) GetLoadBalancers(clusterID string) ([]elbv2types.LoadBalancer, error) {
	args := m.Called(clusterID)
	return args.Get(0).([]elbv2types.LoadBalancer), args.Error(1)
}

func (m *MockAWSClient) DescribeSubnets(input *ec2.DescribeSubnetsInput) (*ec2.DescribeSubnetsOutput, error) {
	args := m.Called(input)
	if out, ok := args.Get(0).(*ec2.DescribeSubnetsOutput); ok {
		return out, args.Error(1)
	}
	return nil, args.Error(1)
}

func TestGetHostedZones(t *testing.T) {

	apiUrl := "example.com"

	tests := []struct {
		name           string
		mockResponses  []*route53.ListHostedZonesOutput
		mockError      error
		expectedResult []route53types.HostedZone
		expectedError  error
	}{
		{
			name: "Success_Matching_HostedZones_with_Pagination",
			mockResponses: []*route53.ListHostedZonesOutput{
				{
					HostedZones: []route53types.HostedZone{
						{Name: aws1.String("test.example.com")},
					},
					NextMarker: aws1.String("next-page-token"),
				},
				{
					HostedZones: []route53types.HostedZone{
						{Name: aws1.String("another.example.com")},
					},
					NextMarker: nil,
				},
			},
			mockError: nil,
			expectedResult: []route53types.HostedZone{
				{Name: aws1.String("test.example.com")},
				{Name: aws1.String("another.example.com")},
			},
			expectedError: nil,
		},
		{
			name: "Error_ListHostedZones_API_Call",
			mockResponses: []*route53.ListHostedZonesOutput{
				nil,
			},
			mockError:      assert.AnError,
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
		{
			name: "No_HostedZones_Matching_apiUrl",
			mockResponses: []*route53.ListHostedZonesOutput{
				{
					HostedZones: []route53types.HostedZone{
						{Name: aws1.String("no-match.com")},
					},
					NextMarker: nil,
				},
			},
			mockError:      nil,
			expectedResult: []route53types.HostedZone{},
			expectedError:  nil,
		},
		{
			name: "No_HostedZones_Returned",
			mockResponses: []*route53.ListHostedZonesOutput{
				{
					HostedZones: []route53types.HostedZone{},
					NextMarker:  nil,
				},
			},
			mockError:      nil,
			expectedResult: []route53types.HostedZone{},
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockAWSClient)

			for i, response := range tt.mockResponses {
				mockClient.On("ListHostedZones", mock.MatchedBy(func(input *route53.ListHostedZonesInput) bool {

					if i == 0 {
						return input.Marker == nil
					}

					return *input.Marker == "next-page-token"
				})).Return(response, tt.mockError)
			}

			result, err := getHostedZones(mockClient, apiUrl)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedResult, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetResourceRecordSets(t *testing.T) {

	tests := []struct {
		name               string
		clusterHostedZones []route53types.HostedZone
		mockResponses      []*route53.ListResourceRecordSetsOutput
		mockError          error
		expectedResult     []route53types.ResourceRecordSet
		expectedError      error
	}{
		{
			name: "Success_Multiple_Hosted_Zones",
			clusterHostedZones: []route53types.HostedZone{
				{Id: aws1.String("zone1")},
				{Id: aws1.String("zone2")},
			},
			mockResponses: []*route53.ListResourceRecordSetsOutput{
				{
					ResourceRecordSets: []route53types.ResourceRecordSet{
						{Name: aws1.String("record1.zone1.com"), Type: route53types.RRTypeA},
					},
				},
				{
					ResourceRecordSets: []route53types.ResourceRecordSet{
						{Name: aws1.String("record1.zone2.com"), Type: route53types.RRTypeA},
					},
				},
			},
			mockError: nil,
			expectedResult: []route53types.ResourceRecordSet{
				{Name: aws1.String("record1.zone1.com"), Type: route53types.RRTypeA},
				{Name: aws1.String("record1.zone2.com"), Type: route53types.RRTypeA},
			},
			expectedError: nil,
		},
		{
			name: "Error_ListResourceRecordSets_API_Call",
			clusterHostedZones: []route53types.HostedZone{
				{Id: aws1.String("zone1")},
			},
			mockResponses:  nil,
			mockError:      assert.AnError,
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
		{
			name:               "Empty_Cluster_Hosted_Zones",
			clusterHostedZones: []route53types.HostedZone{},
			mockResponses:      nil,
			mockError:          nil,
			expectedResult:     nil,
			expectedError:      nil,
		},
		{
			name: "Empty_Resource_Record_Sets",
			clusterHostedZones: []route53types.HostedZone{
				{Id: aws1.String("zone1")},
			},
			mockResponses: []*route53.ListResourceRecordSetsOutput{
				{
					ResourceRecordSets: []route53types.ResourceRecordSet{},
				},
			},
			mockError:      nil,
			expectedResult: []route53types.ResourceRecordSet{},
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockAWSClient)

			for i, response := range tt.mockResponses {

				mockClient.On("ListResourceRecordSets", mock.MatchedBy(func(input *route53.ListResourceRecordSetsInput) bool {

					return *input.HostedZoneId == *tt.clusterHostedZones[i].Id
				})).Return(response, tt.mockError)
			}

			if tt.mockError != nil {
				mockClient.On("ListResourceRecordSets", mock.Anything).Return(nil, tt.mockError)
			}

			result, err := getResourceRecordSets(mockClient, tt.clusterHostedZones)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedResult, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetVpcEndpointConnections(t *testing.T) {
	tests := []struct {
		name           string
		services       []types.ServiceDetail
		mockResponse   *ec2.DescribeVpcEndpointConnectionsOutput
		mockError      error
		expectedResult []types.VpcEndpointConnection
		expectedError  error
	}{
		{
			name: "Success_Multiple_Services",
			services: []types.ServiceDetail{
				{ServiceId: aws1.String("service1")},
				{ServiceId: aws1.String("service2")},
			},
			mockResponse: &ec2.DescribeVpcEndpointConnectionsOutput{
				VpcEndpointConnections: []types.VpcEndpointConnection{
					{
						ServiceId:     aws1.String("service1"),
						VpcEndpointId: aws1.String("vpc-endpoint-1"),
					},
					{
						ServiceId:     aws1.String("service2"),
						VpcEndpointId: aws1.String("vpc-endpoint-2"),
					},
				},
			},
			mockError: nil,
			expectedResult: []types.VpcEndpointConnection{
				{
					ServiceId:     aws1.String("service1"),
					VpcEndpointId: aws1.String("vpc-endpoint-1"),
				},
				{
					ServiceId:     aws1.String("service2"),
					VpcEndpointId: aws1.String("vpc-endpoint-2"),
				},
			},
			expectedError: nil,
		},
		{
			name: "Error_DescribeVpcEndpointConnections_API_Call",
			services: []types.ServiceDetail{
				{ServiceId: aws1.String("service1")},
			},
			mockResponse:   nil,
			mockError:      assert.AnError,
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
		{
			name: "Empty_VPC_Endpoint_Connections",
			services: []types.ServiceDetail{
				{ServiceId: aws1.String("service1")},
			},
			mockResponse: &ec2.DescribeVpcEndpointConnectionsOutput{
				VpcEndpointConnections: []types.VpcEndpointConnection{},
			},
			mockError:      nil,
			expectedResult: []types.VpcEndpointConnection{},
			expectedError:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockAWSClient)

			mockClient.On("DescribeVpcEndpointConnections", mock.MatchedBy(func(input *ec2.DescribeVpcEndpointConnectionsInput) bool {
				expectedServiceIds := []string{}
				for _, service := range tt.services {
					expectedServiceIds = append(expectedServiceIds, *service.ServiceId)
				}
				return len(input.Filters) == 1 && *input.Filters[0].Name == "service-id" && len(input.Filters[0].Values) == len(expectedServiceIds)
			})).Return(tt.mockResponse, tt.mockError)

			result, err := getVpcEndpointConnections(mockClient, tt.services)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedResult, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetVpcEndpoints(t *testing.T) {
	tests := []struct {
		name           string
		clusterID      string
		tagKey         string
		tagValue       string
		mockResponse   *ec2.DescribeVpcEndpointsOutput
		mockError      error
		expectedResult []types.VpcEndpoint
		expectedError  error
	}{
		{
			name:      "Success_VPC_endpoints_match_tags",
			clusterID: "cluster1",
			tagKey:    "Name",
			tagValue:  "test-endpoint",
			mockResponse: &ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []types.VpcEndpoint{
					{
						VpcEndpointId: aws1.String("vpce-1"),
						Tags: []types.Tag{
							{Key: aws1.String("Name"), Value: aws1.String("test-endpoint")},
						},
					},
					{
						VpcEndpointId: aws1.String("vpce-2"),
						Tags: []types.Tag{
							{Key: aws1.String("Name"), Value: aws1.String("test-endpoint")},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: []types.VpcEndpoint{
				{
					VpcEndpointId: aws1.String("vpce-1"),
					Tags: []types.Tag{
						{Key: aws1.String("Name"), Value: aws1.String("test-endpoint")},
					},
				},
				{
					VpcEndpointId: aws1.String("vpce-2"),
					Tags: []types.Tag{
						{Key: aws1.String("Name"), Value: aws1.String("test-endpoint")},
					},
				},
			},
			expectedError: nil,
		},
		{
			name:           "Error_DescribeVpcEndpoints_API_call_fails",
			clusterID:      "cluster1",
			tagKey:         "Name",
			tagValue:       "test-endpoint",
			mockResponse:   nil,
			mockError:      assert.AnError,
			expectedResult: nil,
			expectedError:  assert.AnError,
		},
		{
			name:      "No_VPC endpoints_match_the_tags",
			clusterID: "cluster1",
			tagKey:    "Name",
			tagValue:  "non-existing-endpoint",
			mockResponse: &ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []types.VpcEndpoint{
					{
						VpcEndpointId: aws1.String("vpce-1"),
						Tags: []types.Tag{
							{Key: aws1.String("Name"), Value: aws1.String("different-endpoint")},
						},
					},
				},
			},
			mockError:      nil,
			expectedResult: nil,
			expectedError:  nil,
		},

		{
			name:      "Empty_list_of_VPC_endpoints",
			clusterID: "cluster1",
			tagKey:    "Name",
			tagValue:  "test-endpoint",
			mockResponse: &ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []types.VpcEndpoint{},
			},
			mockError:      nil,
			expectedResult: nil,
			expectedError:  nil,
		},

		{
			name:      "Multiple_VPC_endpoints_match_the_tags",
			clusterID: "cluster1",
			tagKey:    "Environment",
			tagValue:  "production",
			mockResponse: &ec2.DescribeVpcEndpointsOutput{
				VpcEndpoints: []types.VpcEndpoint{
					{
						VpcEndpointId: aws1.String("vpce-1"),
						Tags: []types.Tag{
							{Key: aws1.String("Environment"), Value: aws1.String("production")},
						},
					},
					{
						VpcEndpointId: aws1.String("vpce-2"),
						Tags: []types.Tag{
							{Key: aws1.String("Environment"), Value: aws1.String("production")},
						},
					},
				},
			},
			mockError: nil,
			expectedResult: []types.VpcEndpoint{
				{
					VpcEndpointId: aws1.String("vpce-1"),
					Tags: []types.Tag{
						{Key: aws1.String("Environment"), Value: aws1.String("production")},
					},
				},
				{
					VpcEndpointId: aws1.String("vpce-2"),
					Tags: []types.Tag{
						{Key: aws1.String("Environment"), Value: aws1.String("production")},
					},
				},
			},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockAWSClient)

			mockClient.On("DescribeVpcEndpoints", mock.MatchedBy(func(input *ec2.DescribeVpcEndpointsInput) bool {

				return len(input.Filters) == 0
			})).Return(tt.mockResponse, tt.mockError)

			result, err := getVpcEndpoints(mockClient, tt.clusterID, tt.tagKey, tt.tagValue)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.Nil(t, result)
				assert.Equal(t, tt.expectedError, err)
			} else {
				assert.NoError(t, err)
				assert.ElementsMatch(t, tt.expectedResult, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetVpcEndpointServices(t *testing.T) {
	clusterID := "test-cluster-id"
	tests := []struct {
		name               string
		mockServicesOutput *ec2.DescribeVpcEndpointServicesOutput
		mockError          error
		expectedResult     []types.ServiceDetail
		expectedError      string
	}{
		{
			name: "Success_Matching_cluster_ID",
			mockServicesOutput: &ec2.DescribeVpcEndpointServicesOutput{
				ServiceDetails: []types.ServiceDetail{
					{
						Tags: []types.Tag{
							{
								Key:   aws1.String("api.openshift.com/id"),
								Value: aws1.String("test-cluster-id"),
							},
						},
					},
					{
						Tags: []types.Tag{
							{
								Key:   aws1.String("api.openshift.com/id"),
								Value: aws1.String("other-cluster-id"),
							},
						},
					},
				},
			},
			mockError:      nil,
			expectedResult: []types.ServiceDetail{{Tags: []types.Tag{{Key: aws1.String("api.openshift.com/id"), Value: aws1.String("test-cluster-id")}}}},
			expectedError:  "",
		},
		{
			name: "No_Matching_Services",
			mockServicesOutput: &ec2.DescribeVpcEndpointServicesOutput{
				ServiceDetails: []types.ServiceDetail{
					{
						Tags: []types.Tag{
							{
								Key:   aws1.String("api.openshift.com/id"),
								Value: aws1.String("another-cluster-id"),
							},
						},
					},
				},
			},
			mockError:      nil,
			expectedResult: []types.ServiceDetail{},
			expectedError:  "",
		},
		{
			name:               "Error Fetching Services",
			mockServicesOutput: nil,
			mockError:          errors.New("API call failed"),
			expectedResult:     nil,
			expectedError:      "API call failed",
		},
		{
			name: "Empty_Response",
			mockServicesOutput: &ec2.DescribeVpcEndpointServicesOutput{
				ServiceDetails: []types.ServiceDetail{},
			},
			mockError:      nil,
			expectedResult: []types.ServiceDetail{},
			expectedError:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			mockClient := new(MockAWSClient)
			mockClient.On("DescribeVpcEndpointServices", &ec2.DescribeVpcEndpointServicesInput{}).
				Return(tt.mockServicesOutput, tt.mockError)

			result, err := getVpcEndpointServices(mockClient, clusterID)

			if tt.expectedError != "" {
				assert.NotNil(t, err)
				assert.Equal(t, tt.expectedError, err.Error())
			} else {
				assert.Nil(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestGetLoadBalancers(t *testing.T) {
	mockArn := "arn:aws:elb:region:account:loadbalancer/app/my-lb"
	clusterID := "test-cluster"

	tests := []struct {
		name             string
		clusterID        string
		lbOutput         *elasticloadbalancingv2.DescribeLoadBalancersOutput
		lbErr            error
		tagOutput        *elasticloadbalancingv2.DescribeTagsOutput
		tagErr           error
		expectedErr      string
		expectedLBsCount int
		expectedFirstArn string
	}{
		{
			name:      "success_matching_cluster_ID_in_tags",
			clusterID: clusterID,
			lbOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
				LoadBalancers: []elbv2types.LoadBalancer{
					{LoadBalancerArn: aws1.String(mockArn)},
				},
			},
			tagOutput: &elasticloadbalancingv2.DescribeTagsOutput{
				TagDescriptions: []elbv2types.TagDescription{
					{
						ResourceArn: aws1.String(mockArn),
						Tags: []elbv2types.Tag{
							{
								Key:   aws1.String("kubernetes.io/service-name"),
								Value: aws1.String("svc/" + clusterID),
							},
						},
					},
				},
			},
			expectedLBsCount: 1,
			expectedFirstArn: mockArn,
		},
		{
			name:      "no_matching_cluster_ID_in_tags",
			clusterID: clusterID,
			lbOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
				LoadBalancers: []elbv2types.LoadBalancer{
					{LoadBalancerArn: aws1.String(mockArn)},
				},
			},
			tagOutput: &elasticloadbalancingv2.DescribeTagsOutput{
				TagDescriptions: []elbv2types.TagDescription{
					{
						ResourceArn: aws1.String(mockArn),
						Tags: []elbv2types.Tag{
							{
								Key:   aws1.String("kubernetes.io/service-name"),
								Value: aws1.String("svc/other-cluster"),
							},
						},
					},
				},
			},
			expectedLBsCount: 0,
		},
		{
			name:        "error_from_DescribeLoadBalancers",
			clusterID:   clusterID,
			lbErr:       errors.New("describe LBs failed"),
			expectedErr: "describe LBs failed",
		},
		{
			name:      "error_from_DescribeTags",
			clusterID: clusterID,
			lbOutput: &elasticloadbalancingv2.DescribeLoadBalancersOutput{
				LoadBalancers: []elbv2types.LoadBalancer{
					{LoadBalancerArn: aws1.String(mockArn)},
				},
			},
			tagErr:      errors.New("describe tags failed"),
			expectedErr: "describe tags failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockAWSClient)

			mockClient.On("DescribeV2LoadBalancers", mock.Anything).Return(tt.lbOutput, tt.lbErr)

			if tt.lbErr == nil {

				mockClient.On("DescribeV2Tags", mock.MatchedBy(func(input *elasticloadbalancingv2.DescribeTagsInput) bool {
					return len(input.ResourceArns) > 0 && input.ResourceArns[0] == mockArn
				})).Return(tt.tagOutput, tt.tagErr)
			}

			result, err := getLoadBalancers(mockClient, tt.clusterID)

			if tt.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.Len(t, result, tt.expectedLBsCount)
				if tt.expectedLBsCount > 0 {
					assert.Equal(t, tt.expectedFirstArn, *result[0].LoadBalancerArn)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGatherManagementClusterInfo(t *testing.T) {
	tests := []struct {
		name                    string
		clusterID               string
		mockSetup               func(m *MockAWSClient)
		expectedError           bool
		expectedEndpointCount   int
		expectedConnectionCount int
		expectedLBCount         int
	}{
		{
			name:      "All_resources_found_successfully",
			clusterID: "test-cluster",
			mockSetup: func(m *MockAWSClient) {

				m.On("DescribeVpcEndpointServices", mock.Anything).Return(
					&ec2.DescribeVpcEndpointServicesOutput{
						ServiceNames: []string{*aws1.String("svc-1")},
					}, nil,
				)

				m.On("DescribeVpcEndpointConnections", mock.Anything).Return(
					&ec2.DescribeVpcEndpointConnectionsOutput{
						VpcEndpointConnections: []types.VpcEndpointConnection{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)

				m.On("DescribeV2LoadBalancers", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []elbv2types.LoadBalancer{
							{LoadBalancerArn: aws1.String("arn:aws:elb:lb-1")},
						},
					}, nil,
				)

				m.On("DescribeV2Tags", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []elbv2types.TagDescription{
							{
								ResourceArn: aws1.String("arn:aws:elb:lb-1"),
								Tags: []elbv2types.Tag{
									{
										Key:   aws1.String("kubernetes.io/service-name"),
										Value: aws1.String("svc-test-cluster"),
									},
								},
							},
						},
					}, nil,
				)
			},
			expectedError:           false,
			expectedEndpointCount:   1,
			expectedConnectionCount: 1,
			expectedLBCount:         1,
		},
		{
			name:      "DescribeVpcEndpointServices_fails",
			clusterID: "test-cluster",
			mockSetup: func(m *MockAWSClient) {
				m.On("DescribeVpcEndpointServices", mock.Anything).Return(nil, errors.New("fail"))
				m.On("DescribeVpcEndpointConnections", mock.Anything).Return(
					&ec2.DescribeVpcEndpointConnectionsOutput{
						VpcEndpointConnections: []types.VpcEndpointConnection{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)
				m.On("DescribeV2LoadBalancers", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []elbv2types.LoadBalancer{
							{LoadBalancerArn: aws1.String("arn:aws:elb:lb-1")},
						},
					}, nil,
				)
				m.On("DescribeV2Tags", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []elbv2types.TagDescription{
							{
								ResourceArn: aws1.String("arn:aws:elb:lb-1"),
								Tags: []elbv2types.Tag{
									{
										Key:   aws1.String("kubernetes.io/service-name"),
										Value: aws1.String("svc-test-cluster"),
									},
								},
							},
						},
					}, nil,
				)

			},

			expectedError: true,
		},
		{
			name:      "DescribeVpcEndpointConnections_fails",
			clusterID: "test-cluster",
			mockSetup: func(m *MockAWSClient) {
				m.On("DescribeVpcEndpointServices", mock.Anything).Return(
					&ec2.DescribeVpcEndpointServicesOutput{
						ServiceNames: []string{*aws1.String("svc-1")},
					}, nil,
				)
				m.On("DescribeVpcEndpointConnections", mock.Anything).Return(&ec2.DescribeVpcEndpointConnectionsOutput{}, errors.New("fail"))
				m.On("DescribeV2LoadBalancers", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeLoadBalancersOutput{
						LoadBalancers: []elbv2types.LoadBalancer{
							{LoadBalancerArn: aws1.String("arn:aws:elb:lb-1")},
						},
					}, nil,
				)
				m.On("DescribeV2Tags", mock.Anything).Return(
					&elasticloadbalancingv2.DescribeTagsOutput{
						TagDescriptions: []elbv2types.TagDescription{
							{
								ResourceArn: aws1.String("arn:aws:elb:lb-1"),
								Tags: []elbv2types.Tag{
									{
										Key:   aws1.String("kubernetes.io/service-name"),
										Value: aws1.String("svc-test-cluster"),
									},
								},
							},
						},
					}, nil,
				)
			},
			expectedError: true,
		},
		{
			name:      "DescribeV2LoadBalancers_fails",
			clusterID: "test-cluster",
			mockSetup: func(m *MockAWSClient) {
				m.On("DescribeVpcEndpointServices", mock.Anything).Return(
					&ec2.DescribeVpcEndpointServicesOutput{
						ServiceNames: []string{*aws1.String("svc-1")},
					}, nil,
				)
				m.On("DescribeVpcEndpointConnections", mock.Anything).Return(
					&ec2.DescribeVpcEndpointConnectionsOutput{
						VpcEndpointConnections: []types.VpcEndpointConnection{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)
				m.On("DescribeV2LoadBalancers", mock.Anything).Return(&elasticloadbalancingv2.DescribeLoadBalancersOutput{}, errors.New("fail"))
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockAWSClient)
			tt.mockSetup(mockClient)

			ch := make(chan ChanReturn[managementClusterInfo], 1)
			go gatherManagementClusterInfo(mockClient, tt.clusterID, ch)

			result := <-ch

			if tt.expectedError {
				assert.Error(t, result.Error)
			} else {
				assert.NoError(t, result.Error)
				assert.Len(t, result.Value.EndpointConnections, tt.expectedConnectionCount)
				assert.Len(t, result.Value.LoadBalancers, tt.expectedLBCount)
			}
		})
	}
}

func newTestCluster(t *testing.T, cb *v1.ClusterBuilder) *v1.Cluster {
	cluster, err := cb.Build()
	if err != nil {
		t.Fatalf("failed to build cluster: %s", err)
	}

	return cluster
}

func TestGatherPrivatelinkClusterInfo(t *testing.T) {
	tests := []struct {
		name        string
		mockSetup   func(m *MockAWSClient)
		cluster     *v1.Cluster
		expectedErr bool
	}{

		{
			name:    "success_case",
			cluster: newTestCluster(t, v1.NewCluster().CloudProvider(v1.NewCloudProvider().ID("aws"))),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(&route53.ListHostedZonesOutput{
					HostedZones: []route53types.HostedZone{{Id: aws1.String("hz-1"), Name: aws1.String("test-cluster")}},
				}, nil)
				m.On("ListResourceRecordSets", mock.Anything).Return(&route53.ListResourceRecordSetsOutput{
					ResourceRecordSets: []route53types.ResourceRecordSet{{Name: aws1.String("api.test-cluster")}},
				}, nil)
				m.On("DescribeVpcEndpoints", mock.Anything).Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []types.VpcEndpoint{{VpcEndpointId: aws1.String("vpce-1")}},
				}, nil)
			},
			expectedErr: false,
		},
		{
			name:    "ListHostedZones_fails",
			cluster: newTestCluster(t, v1.NewCluster().CloudProvider(v1.NewCloudProvider().ID("aws"))),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(&route53.ListHostedZonesOutput{}, errors.New("ListHostedZones failed"))
				m.On("DescribeVpcEndpoints", mock.Anything).Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []types.VpcEndpoint{{VpcEndpointId: aws1.String("vpce-1")}},
				}, nil)
			},
			expectedErr: true,
		},
		{
			name:    "ListResourceRecordSets_fails",
			cluster: newTestCluster(t, v1.NewCluster().CloudProvider(v1.NewCloudProvider().ID("aws"))),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(&route53.ListHostedZonesOutput{
					HostedZones: []route53types.HostedZone{{Id: aws1.String("hz-1"), Name: aws1.String("test-cluster")}},
				}, nil)
				m.On("ListResourceRecordSets", mock.Anything).Return(nil, errors.New("ListResourceRecordSets failed"))
				m.On("DescribeVpcEndpoints", mock.Anything).Return(&ec2.DescribeVpcEndpointsOutput{
					VpcEndpoints: []types.VpcEndpoint{{VpcEndpointId: aws1.String("vpce-1")}},
				}, nil)
			},
			expectedErr: true,
		},
		{
			name:    "DescribeVpcEndpoints_fails",
			cluster: newTestCluster(t, v1.NewCluster().CloudProvider(v1.NewCloudProvider().ID("aws"))),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(&route53.ListHostedZonesOutput{
					HostedZones: []route53types.HostedZone{{Id: aws1.String("hz-1"), Name: aws1.String("test-cluster")}},
				}, nil)
				m.On("ListResourceRecordSets", mock.Anything).Return(&route53.ListResourceRecordSetsOutput{
					ResourceRecordSets: []route53types.ResourceRecordSet{{Name: aws1.String("api.test-cluster")}},
				}, nil)
				m.On("DescribeVpcEndpoints", mock.Anything).Return(&ec2.DescribeVpcEndpointsOutput{}, errors.New("DescribeVpcEndpoints failed"))
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAWS := new(MockAWSClient)
			tt.mockSetup(mockAWS)

			ch := make(chan ChanReturn[privatelinkInfo], 1)
			go gatherPrivatelinkClusterInfo(mockAWS, tt.cluster, ch)

			result := <-ch
			if (result.Error != nil) != tt.expectedErr {
				t.Errorf("unexpected error state: got error=%v, expectedErr=%v", result.Error, tt.expectedErr)
			}
		})
	}
}

func TestGetSubnets(t *testing.T) {
	tests := []struct {
		name      string
		subnetIDs []string
		mockSetup func(m *MockAWSClient)
		expected  []types.Subnet
	}{
		{
			name:      "successfully_returns_subnets",
			subnetIDs: []string{"subnet-1", "subnet-2"},
			mockSetup: func(m *MockAWSClient) {
				m.On("DescribeSubnets", &ec2.DescribeSubnetsInput{
					SubnetIds: []string{"subnet-1", "subnet-2"},
				}).Return(&ec2.DescribeSubnetsOutput{
					Subnets: []types.Subnet{
						{SubnetId: aws1.String("subnet-1")},
						{SubnetId: aws1.String("subnet-2")},
					},
				}, nil)
			},
			expected: []types.Subnet{
				{SubnetId: aws1.String("subnet-1")},
				{SubnetId: aws1.String("subnet-2")},
			},
		},
		{
			name:      "DescribeSubnets_returns_error",
			subnetIDs: []string{"subnet-3"},
			mockSetup: func(m *MockAWSClient) {
				m.On("DescribeSubnets", &ec2.DescribeSubnetsInput{
					SubnetIds: []string{"subnet-3"},
				}).Return(nil, errors.New("AWS error"))
			},
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(MockAWSClient)
			tt.mockSetup(mockClient)

			result, err := getSubnets(mockClient, tt.subnetIDs)

			if err != nil {
				assert.Nil(t, result)
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGatherCustomerClusterInfo(t *testing.T) {
	tests := []struct {
		name              string
		mockSetup         func(m *MockAWSClient)
		cluster           *v1.Cluster
		expectedError     bool
		expectedZoneCount int
	}{
		{
			name:    "successfully_gathers_all_info",
			cluster: newTestCluster(t, v1.NewCluster().ID("test-cluster").Name("test")),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(
					&route53.ListHostedZonesOutput{
						HostedZones: []route53types.HostedZone{
							{Id: aws1.String("zone-1"), Name: aws1.String("test-cluster")},
						},
					}, nil,
				)
				m.On("ListResourceRecordSets", mock.Anything).Return(
					&route53.ListResourceRecordSetsOutput{
						ResourceRecordSets: []route53types.ResourceRecordSet{
							{Name: aws1.String("record")},
						},
					}, nil,
				)
				m.On("DescribeVpcEndpoints", mock.Anything).Return(
					&ec2.DescribeVpcEndpointsOutput{
						VpcEndpoints: []types.VpcEndpoint{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)
				m.On("DescribeSubnets", mock.Anything).Return(
					&ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{SubnetId: aws1.String("subnet-1")},
						},
					}, nil,
				)
				m.On("DescribeRouteTables", mock.Anything).Return(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{RouteTableId: aws1.String("rtb-1")},
						},
					}, nil,
				)
			},
			expectedError:     false,
			expectedZoneCount: 1,
		},
		{
			name:    "hosted_zone_error",
			cluster: newTestCluster(t, v1.NewCluster().ID("fail-cluster").Name("fail")),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(&route53.ListHostedZonesOutput{}, errors.New("fail hosted zones"))
				m.On("DescribeVpcEndpoints", mock.Anything).Return(
					&ec2.DescribeVpcEndpointsOutput{
						VpcEndpoints: []types.VpcEndpoint{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)
				m.On("DescribeSubnets", mock.Anything).Return(
					&ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{SubnetId: aws1.String("subnet-1")},
						},
					}, nil,
				)
				m.On("DescribeRouteTables", mock.Anything).Return(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{RouteTableId: aws1.String("rtb-1")},
						},
					}, nil,
				)

			},
			expectedError: true,
		},
		{
			name:    "resource_record_set_error",
			cluster: newTestCluster(t, v1.NewCluster().ID("rrs-fail").Name("rrs")),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(
					&route53.ListHostedZonesOutput{
						HostedZones: []route53types.HostedZone{{Id: aws1.String("hz-1"), Name: aws1.String("test-cluster")}},
					}, nil,
				)
				m.On("ListResourceRecordSets", mock.Anything).Return(nil, errors.New("rrs fail"))
				m.On("DescribeVpcEndpoints", mock.Anything).Return(
					&ec2.DescribeVpcEndpointsOutput{
						VpcEndpoints: []types.VpcEndpoint{
							{VpcEndpointId: aws1.String("ep-1")},
						},
					}, nil,
				)
				m.On("DescribeSubnets", mock.Anything).Return(
					&ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{SubnetId: aws1.String("subnet-1")},
						},
					}, nil,
				)
				m.On("DescribeRouteTables", mock.Anything).Return(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{RouteTableId: aws1.String("rtb-1")},
						},
					}, nil,
				)
			},
			expectedError: true,
		},
		{
			name:    "vpc_endpoint_error",
			cluster: newTestCluster(t, v1.NewCluster().ID("vpce-fail").Name("vpce")),
			mockSetup: func(m *MockAWSClient) {
				m.On("ListHostedZones", mock.Anything).Return(
					&route53.ListHostedZonesOutput{
						HostedZones: []route53types.HostedZone{{Id: aws1.String("hz-1"), Name: aws1.String("test-cluster2")}},
					}, nil,
				)
				m.On("ListResourceRecordSets", mock.Anything).Return(
					&route53.ListResourceRecordSetsOutput{
						ResourceRecordSets: []route53types.ResourceRecordSet{},
					}, nil,
				)
				m.On("DescribeVpcEndpoints", mock.Anything).Return(&ec2.DescribeVpcEndpointsOutput{}, errors.New("vpce fail"))
				m.On("DescribeSubnets", mock.Anything).Return(
					&ec2.DescribeSubnetsOutput{
						Subnets: []types.Subnet{
							{SubnetId: aws1.String("subnet-1")},
						},
					}, nil,
				)
				m.On("DescribeRouteTables", mock.Anything).Return(
					&ec2.DescribeRouteTablesOutput{
						RouteTables: []types.RouteTable{
							{RouteTableId: aws1.String("rtb-1")},
						},
					}, nil,
				)
			},
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAWS := new(MockAWSClient)
			if tt.mockSetup != nil {
				tt.mockSetup(mockAWS)
			}

			ch := make(chan ChanReturn[clusterInfo], 1)
			go gatherCustomerClusterInfo(mockAWS, tt.cluster, ch)

			result := <-ch
			if tt.expectedError {
				assert.Error(t, result.Error)
			} else {
				assert.NoError(t, result.Error)
				assert.Equal(t, tt.expectedZoneCount, len(result.Value.HostedZones))
			}
		})
	}
}

func TestCreateGraphViz(t *testing.T) {
	tests := []struct {
		name            string
		input           *aggregateClusterInfo
		expectedNodeIds []string
		expectedEdges   [][2]string
	}{
		{
			name: "Basic_Graph_Creation",
			input: &aggregateClusterInfo{
				privatelinkInfo: &privatelinkInfo{
					HostedZones: []route53types.HostedZone{
						{
							Id:   aws1.String("pl-hz-1"),
							Name: aws1.String("vpce.test.internal."),
							Config: &route53types.HostedZoneConfig{
								PrivateZone: true,
							},
						},
					},
					ResourceRecords: []route53types.ResourceRecordSet{
						{
							Name: aws1.String("rr1"),
							ResourceRecords: []route53types.ResourceRecord{
								{
									Value: aws1.String("vpce-abc123.us-west-2.vpce.amazonaws.com"),
								},
							},
						},
					},
				},
				managementClusterInfo: &managementClusterInfo{
					EndpointServices: []types.ServiceDetail{
						{
							ServiceId: aws1.String("svc-123"),
							BaseEndpointDnsNames: []string{
								"vpce-abc123.us-west-2.vpce.amazonaws.com",
							},
							ServiceType: []types.ServiceTypeDetail{
								{ServiceType: types.ServiceTypeInterface},
							},
							ServiceName: aws1.String("com.amazonaws.vpce.svc"),
							Owner:       aws1.String("owner"),
						},
					},
					EndpointConnections: []types.VpcEndpointConnection{
						{
							VpcEndpointConnectionId: aws1.String("conn-123"),
							VpcEndpointId:           aws1.String("ep-123"),
							VpcEndpointOwner:        aws1.String("owner"),
							IpAddressType:           types.IpAddressTypeIpv4,
							VpcEndpointState:        types.StateAvailable,
							DnsEntries: []types.DnsEntry{
								{DnsName: aws1.String("dns-123")},
							},
							NetworkLoadBalancerArns: []string{"lb-arn-123"},
						},
					},
				},
				clusterInfo: &clusterInfo{
					HostedZones: []route53types.HostedZone{
						{
							Id:   aws1.String("cust-hz-1"),
							Name: aws1.String("example.com."),
							Config: &route53types.HostedZoneConfig{
								PrivateZone: true,
							},
						},
					},
					ResourceRecords: []route53types.ResourceRecordSet{
						{
							Name: aws1.String("api.example.com."),
							ResourceRecords: []route53types.ResourceRecord{
								{
									Value: aws1.String("vpce.test.internal"),
								},
							},
						},
					},
					Endpoints: []types.VpcEndpoint{
						{
							VpcEndpointId:   aws1.String("ep-123"),
							VpcId:           aws1.String("vpc-123"),
							ServiceName:     aws1.String("svc-name"),
							State:           types.State(types.VpcStateAvailable),
							VpcEndpointType: types.VpcEndpointTypeInterface,
						},
					},
				},
			},
			expectedNodeIds: []string{
				"pl-hz-1",
				"vpce-abc123.us-west-2.vpce.amazonaws.com",
				"svc-123",
				"conn-123",
				"lb-arn-123",
				"ep-123",
				"cust-hz-1",
				"vpce.test.internal",
			},
			expectedEdges: [][2]string{
				{"vpce-abc123.us-west-2.vpce.amazonaws.com", "svc-123"},
				{"svc-123", "conn-123"},
				{"conn-123", "lb-arn-123"},
				{"conn-123", "ep-123"},
				{"pl-hz-1", "vpce.test.internal"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := createGraphViz(tt.input)

			nodeIdSet := make(map[string]struct{})
			for from, toList := range graph {
				nodeIdSet[from.Id] = struct{}{}
				for _, to := range toList {
					nodeIdSet[to.Id] = struct{}{}
				}
			}

			for _, nodeId := range tt.expectedNodeIds {
				_, found := nodeIdSet[nodeId]
				assert.True(t, found, "Expected node ID %q not found in graph", nodeId)
			}

			for _, edge := range tt.expectedEdges {
				from := edge[0]
				to := edge[1]
				found := false
				for node, neighbors := range graph {
					if node.Id == from {
						for _, neighbor := range neighbors {
							if neighbor.Id == to {
								found = true
								break
							}
						}
					}
				}
				assert.True(t, found, "Expected edge from %q to %q not found", from, to)
			}
		})
	}
}
