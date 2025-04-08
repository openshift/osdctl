package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbv2types "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/olekukonko/tablewriter"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/graphviz"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const HYPERSHIFT_URL = "/api/clusters_mgmt/v1/clusters/{cluster_id}/hypershift"

var verbose bool = false

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func NewCmdHypershiftInfo(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newInfoOptions(streams)
	infoCmd := &cobra.Command{
		Use:   "hypershift-info",
		Short: "Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster",
		Long: `This command aggregates AWS objects from the cluster, management cluster and privatelink for hypershift cluster.
It attempts to render the relationships as graphviz if that output format is chosen or will simply print the output as tables.`,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}
	infoCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "c", "", "Provide internal ID of the cluster")
	infoCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	infoCmd.Flags().StringVarP(&ops.awsRegion, "region", "r", "", "AWS Region")
	infoCmd.Flags().StringVarP(&ops.privatelinkAccountId, "privatelinkaccount", "l", "", "Privatelink account ID")
	infoCmd.Flags().StringVarP(&ops.output, "output", "o", "graphviz", "output format ['table', 'graphviz']")
	infoCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	// Mark cluster-id as required
	infoCmd.MarkFlagRequired("cluster-id")

	return infoCmd
}

// infoOptions defines the struct for running the hypershift info command
type infoOptions struct {
	clusterID            string
	awsProfile           string
	awsRegion            string
	privatelinkAccountId string
	output               string
	verbose              bool
	genericclioptions.IOStreams
}

// InfoClusters aggregates the OCM responses for the customer and management
// cluster.
type infoClusters struct {
	managementCluster *v1.Cluster
	customerCluster   *v1.Cluster
}

// Store all clients required to access the 3 different AWS accounts
type hypershiftAWSClients struct {
	customerClient    aws.Client
	managementClient  aws.Client
	privatelinkClient aws.Client
}

// Information related to the customer cluster
type clusterInfo struct {
	HostedZones       []route53types.HostedZone
	ResourceRecords   []route53types.ResourceRecordSet
	Endpoints         []ec2types.VpcEndpoint
	Subnets           []ec2types.Subnet
	SubnetRouteTables []ec2types.RouteTable
}

// Information related to the management cluster
type managementClusterInfo struct {
	EndpointConnections []ec2types.VpcEndpointConnection
	EndpointServices    []ec2types.ServiceDetail
	LoadBalancers       []elbv2types.LoadBalancer
}

// Information related to the privatelink account
type privatelinkInfo struct {
	HostedZones     []route53types.HostedZone
	ResourceRecords []route53types.ResourceRecordSet
	Endpoints       []ec2types.VpcEndpoint
}

// Aggregates all 3 AWS account informations in one
type aggregateClusterInfo struct {
	lock                  sync.Mutex
	managementClusterInfo *managementClusterInfo
	clusterInfo           *clusterInfo
	privatelinkInfo       *privatelinkInfo
}

type retrievable interface {
	clusterInfo | managementClusterInfo | privatelinkInfo
}

type ChanReturn[T retrievable] struct {
	Value T
	Error error
}

func newInfoOptions(streams genericclioptions.IOStreams) *infoOptions {
	return &infoOptions{
		IOStreams: streams,
	}
}

func (i *infoOptions) complete(cmd *cobra.Command) error {
	var errMsg string

	if i.awsProfile == "" {
		errMsg += "missing argument -p. "
	}
	if i.privatelinkAccountId == "" {
		errMsg += "missing argument -l."
	}
	if i.output != "" {
		if i.output != "graphviz" && i.output != "table" {
			errMsg += "output must be 'graphviz' or 'table'"
		}
	}
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}
	return nil
}

func baseApiUrl(c *v1.Cluster) string {
	apiUrlSplit := strings.Split(c.API().URL(), ":")
	clusterPort := fmt.Sprintf(":%s", apiUrlSplit[len(apiUrlSplit)-1])
	return strings.Replace(strings.Replace(strings.Replace(c.API().URL(), "https://api.", "", 1), c.Name(), "", 1), clusterPort, "", 1)
}

func (i *infoOptions) run() error {
	if i.verbose {
		verbose = true

	}
	verboseLog("Getting hypershift info for cluster: ", i.clusterID)
	clusters, err := i.getClusters()
	if err != nil {
		return err
	}
	if i.awsRegion == "" {
		i.awsRegion = clusters.customerCluster.Region().ID()
		verboseLog("Set aws region to: ", i.awsRegion)
	}

	verboseLog("Constructing AWS sessions for all accounts")
	awsSessions, err := i.getAWSSessions(clusters)
	if err != nil {
		fmt.Println("Could not construct all AWS sessions: ", err)
		return err
	}
	ai := aggregateClusterInfo{}
	plC := make(chan ChanReturn[privatelinkInfo])
	mcC := make(chan ChanReturn[managementClusterInfo])
	cC := make(chan ChanReturn[clusterInfo])
	// To find hostedzones we use the api-url but strip the 'api' and 'clustername' parts
	verboseLog("Retrieving resources from accounts")
	go gatherManagementClusterInfo(awsSessions.managementClient, clusters.customerCluster.ID(), mcC)
	go gatherCustomerClusterInfo(awsSessions.customerClient, clusters.customerCluster, cC)
	go gatherPrivatelinkClusterInfo(awsSessions.privatelinkClient, clusters.customerCluster, plC)
	for {
		select {
		case r := <-plC:
			verboseLog("Received privatelink information")
			if r.Error != nil {
				return r.Error
			}
			ai.privatelinkInfo = &r.Value
		case r := <-mcC:
			verboseLog("Received management information")
			if r.Error != nil {
				return r.Error
			}
			ai.managementClusterInfo = &r.Value
		case r := <-cC:
			verboseLog("Received customer information")
			if r.Error != nil {
				return r.Error
			}
			ai.clusterInfo = &r.Value
		}
		if ai.clusterInfo != nil && ai.managementClusterInfo != nil && ai.privatelinkInfo != nil {
			break
		}
	}
	switch i.output {
	case "table":
		render(&ai)
	case "graphviz":
		connections := createGraphViz(&ai)
		verboseLog("Generating GraphViz Input - please run this: 'echo <output> | dot -Tpng -o/tmp/example.png'")
		graphviz.RenderGraphViz(connections)
	default:
		fmt.Println("No valid output format selected")
	}
	return nil
}

func createGraphViz(ai *aggregateClusterInfo) map[graphviz.Node][]graphviz.Node {
	connections := make(map[graphviz.Node][]graphviz.Node)
	for _, hz := range ai.privatelinkInfo.HostedZones {
		hzn := graphviz.Node{
			Id:                    *hz.Id,
			AdditionalInformation: fmt.Sprintf("Hosted Zone (P)\\n%s", *hz.Name),
			Subgraph:              "privatelink",
		}
		connections[hzn] = make([]graphviz.Node, 0)
	}
	var privatelinkVpce string
	for _, rrs := range ai.privatelinkInfo.ResourceRecords {
		for _, rr := range rrs.ResourceRecords {
			if strings.Contains(*rr.Value, "vpce") {
				privatelinkVpce = *rr.Value
			}
		}
	}
	var mgmntService string
	for _, svcs := range ai.managementClusterInfo.EndpointServices {
		for _, dns := range svcs.BaseEndpointDnsNames {
			if strings.Contains(privatelinkVpce, dns) {
				mgmntService = *svcs.ServiceId
			}
		}
	}
	plvpce := graphviz.Node{
		Id:                    privatelinkVpce,
		AdditionalInformation: "VPC Endpoint (P)",
		Subgraph:              "privatelink",
	}
	mgmntsvc := graphviz.Node{
		Id:                    mgmntService,
		AdditionalInformation: "Endpoint Service (M)",
		Subgraph:              "management",
	}
	connections[plvpce] = append(connections[plvpce], mgmntsvc)
	for _, conn := range ai.managementClusterInfo.EndpointConnections {
		node := graphviz.Node{
			Id:                    *conn.VpcEndpointConnectionId,
			AdditionalInformation: "Endpoint Connection (M)",
			Subgraph:              "management",
		}
		connections[mgmntsvc] = append(connections[mgmntsvc], node)
		for _, lb := range conn.NetworkLoadBalancerArns {
			lb := graphviz.Node{
				Id:                    lb,
				AdditionalInformation: "Load Balancer (M)",
				Subgraph:              "management",
			}
			connections[node] = append(connections[node], lb)
		}
		for _, ceps := range ai.clusterInfo.Endpoints {
			customerEndpointNode := graphviz.Node{
				Id:                    *ceps.VpcEndpointId,
				AdditionalInformation: "VPC Endpoint (C)",
				Subgraph:              "customer",
			}
			if *ceps.VpcEndpointId == *conn.VpcEndpointId {
				connections[node] = append(connections[node], customerEndpointNode)
			}
		}
	}
	for _, hz := range ai.clusterInfo.HostedZones {
		hzn := graphviz.Node{
			Id:                    *hz.Id,
			AdditionalInformation: fmt.Sprintf("Hosted Zone (C)\\n%s", *hz.Name),
			Subgraph:              "customer",
		}
		connections[hzn] = make([]graphviz.Node, 0)
	}
	for _, rrs := range ai.clusterInfo.ResourceRecords {
		for _, hz := range ai.privatelinkInfo.HostedZones {
			var hzNode graphviz.Node
			for connection := range connections {
				if connection.Id == *hz.Id {
					hzNode = connection
				}
			}
			for _, rr := range rrs.ResourceRecords {
				if *rr.Value+"." == *hz.Name {
					node := graphviz.Node{
						Id:                    *rr.Value,
						AdditionalInformation: "Resource Record (C)",
						Subgraph:              "customer",
					}
					connections[node] = make([]graphviz.Node, 0)
					connections[hzNode] = append(connections[hzNode], node)
				}
			}
		}
	}
	return connections
}

func (i *infoOptions) getClusters() (*infoClusters, error) {
	ocmConnection, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	clusters := utils.GetClusters(ocmConnection, []string{i.clusterID})
	if len(clusters) != 1 {
		errMsg := fmt.Sprint("Did not find cluster with id ", i.clusterID)
		fmt.Println(errMsg)
		return nil, errors.New(errMsg)
	}
	customerCluster := clusters[0]
	if !customerCluster.Hypershift().Enabled() {
		errMsg := fmt.Sprint("Cluster is not a hypershift cluster")
		fmt.Println(errMsg)
		return nil, errors.New(errMsg)
	}

	type hypershiftresponse struct {
		Enabled           bool   `json:"enabled"`
		ManagementCluster string `json:"management_cluster"`
		HcpNamespace      string `json:"hcp_namespace"`
	}
	hypershiftApi := strings.Replace(HYPERSHIFT_URL, "{cluster_id}", i.clusterID, 1)
	request := ocmConnection.Get()
	request.Path(hypershiftApi)
	response, err := request.Send()
	if err != nil {
		return nil, err
	}
	var hrsp hypershiftresponse
	err = json.Unmarshal(response.Bytes(), &hrsp)
	if err != nil {
		fmt.Println("Could not retrieve management cluster for cluster.")
	}

	clusters = utils.GetClusters(ocmConnection, []string{hrsp.ManagementCluster})
	if len(clusters) != 1 {
		errMsg := fmt.Sprint("Did not find management cluster with id ", hrsp.ManagementCluster)
		fmt.Println(errMsg)
		return nil, errors.New(errMsg)
	}
	managementCluster := clusters[0]
	return &infoClusters{
		managementCluster: managementCluster,
		customerCluster:   customerCluster,
	}, nil
}

func (i *infoOptions) getAWSSessions(clusters *infoClusters) (*hypershiftAWSClients, error) {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer ocmClient.Close()

	customerConfig, err := osdCloud.CreateAWSV2Config(ocmClient, clusters.customerCluster)
	// We have to overwrite the fact that backplane just mangled our configuration.
	// TODO: Do not use the global configuration instead (https://issues.redhat.com/browse/OSD-19773)
	osdctlConfig.EnsureConfigFile()
	if err != nil {
		return nil, err
	}
	customerCreds, err := customerConfig.Credentials.Retrieve(context.TODO())
	if err != nil {
		return nil, err
	}
	customerClient, err := aws.NewAwsClientWithInput(&aws.ClientInput{
		AccessKeyID:     customerCreds.AccessKeyID,
		SecretAccessKey: customerCreds.SecretAccessKey,
		SessionToken:    customerCreds.SessionToken,
		Region:          customerConfig.Region,
	})
	if err != nil {
		return nil, err
	}
	managementConfig, err := osdCloud.CreateAWSV2Config(ocmClient, clusters.managementCluster)
	if err != nil {
		return nil, err
	}
	// We have to overwrite the fact that backplane just mangled our configuration.
	// TODO: Do not use the global configuration instead (https://issues.redhat.com/browse/OSD-19773)
	osdctlConfig.EnsureConfigFile()
	managementCreds, err := managementConfig.Credentials.Retrieve(context.TODO())
	if err != nil {
		return nil, err
	}
	managementClient, err := aws.NewAwsClientWithInput(&aws.ClientInput{
		AccessKeyID:     managementCreds.AccessKeyID,
		SecretAccessKey: managementCreds.SecretAccessKey,
		SessionToken:    managementCreds.SessionToken,
		Region:          managementConfig.Region,
	})
	if err != nil {
		return nil, err
	}
	awsClient, err := aws.NewAwsClient(i.awsProfile, i.awsRegion, "")
	if err != nil {
		verboseLog(fmt.Sprintf("Could not build AWS Client: %s\n", err))
		return nil, err
	}
	// Get the right partition for the final ARN
	partition, err := aws.GetAwsPartition(awsClient)
	if err != nil {
		return nil, err
	}

	// Generate a session name using the SRE's kerberos ID
	sessionName, err := osdCloud.GenerateRoleSessionName(awsClient)
	if err != nil {
		verboseLog(fmt.Sprintf("Could not generate Session Name: %s\n", err))
		return nil, err
	}

	// By default, the target role arn is OrganizationAccountAccessRole (works for -i and non-CCS clusters)
	targetRoleArnString := aws.GenerateRoleARN(i.privatelinkAccountId, osdCloud.OrganizationAccountAccessRole)

	targetRoleArn, err := arn.Parse(targetRoleArnString)
	if err != nil {
		return nil, err
	}

	targetRoleArn.Partition = partition

	assumedRoleCreds, err := osdCloud.GenerateOrganizationAccountAccessCredentials(awsClient, i.privatelinkAccountId, sessionName, partition)
	if err != nil {
		verboseLog(fmt.Sprintf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err))
		return nil, err
	}

	privatelinkClient, err := aws.NewAwsClientWithInput(&aws.ClientInput{
		AccessKeyID:     *assumedRoleCreds.AccessKeyId,
		SecretAccessKey: *assumedRoleCreds.SecretAccessKey,
		SessionToken:    *assumedRoleCreds.SessionToken,
		Region:          i.awsRegion,
	})
	if err != nil {
		return nil, err
	}
	return &hypershiftAWSClients{
		customerClient:    customerClient,
		managementClient:  managementClient,
		privatelinkClient: privatelinkClient,
	}, nil
}

func getHostedZones(client aws.Client, apiUrl string) ([]route53types.HostedZone, error) {
	verboseLog(fmt.Sprintf("Looking for hostedzones with apiURL: %s", apiUrl))
	clusterHostedZones := make([]route53types.HostedZone, 0, 1)
	var nextMarker *string
	for {
		hostedZones, err := client.ListHostedZones(&route53.ListHostedZonesInput{
			Marker: nextMarker,
		})
		if err != nil {
			return nil, err
		}
		for _, hz := range hostedZones.HostedZones {
			if strings.Contains(*hz.Name, apiUrl) {
				clusterHostedZones = append(clusterHostedZones, hz)
			}
		}
		if hostedZones.NextMarker == nil {
			break
		}
		verboseLog("Paginating HostedZones")
		nextMarker = hostedZones.NextMarker
	}
	return clusterHostedZones, nil
}

func getResourceRecordSets(client aws.Client, clusterHostedZones []route53types.HostedZone) ([]route53types.ResourceRecordSet, error) {
	var rrs []route53types.ResourceRecordSet
	for _, hostedZone := range clusterHostedZones {
		input := route53.ListResourceRecordSetsInput{
			HostedZoneId: hostedZone.Id,
		}
		rrsOutput, err := client.ListResourceRecordSets(&input)
		if err != nil {
			return nil, err
		}
		rrs = append(rrs, rrsOutput.ResourceRecordSets...)
	}
	return rrs, nil
}

func getVpcEndpointConnections(client aws.Client, services []ec2types.ServiceDetail) ([]ec2types.VpcEndpointConnection, error) {
	serviceIds := make([]string, 0, len(services))
	for _, service := range services {
		serviceIds = append(serviceIds, *service.ServiceId)
	}
	key := "service-id"
	input := ec2.DescribeVpcEndpointConnectionsInput{
		Filters: []ec2types.Filter{
			{
				Name:   &key,
				Values: serviceIds,
			}},
	}
	connections, err := client.DescribeVpcEndpointConnections(&input)
	if err != nil {
		return nil, err
	}
	return connections.VpcEndpointConnections, nil
}

func getVpcEndpoints(client aws.Client, clusterID, t, v string) ([]ec2types.VpcEndpoint, error) {
	endpoints, err := client.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{})
	if err != nil {
		return nil, err
	}
	clusterEndpoints := make([]ec2types.VpcEndpoint, 0)
	for _, endpoint := range endpoints.VpcEndpoints {
		for _, tag := range endpoint.Tags {
			if *tag.Key == t && *tag.Value == v {
				clusterEndpoints = append(clusterEndpoints, endpoint)
			}
		}
	}
	return clusterEndpoints, nil
}

func getVpcEndpointServices(client aws.Client, clusterID string) ([]ec2types.ServiceDetail, error) {
	clusterEndpointConnections := make([]ec2types.ServiceDetail, 0, 1)
	servicesInput := ec2.DescribeVpcEndpointServicesInput{}
	services, err := client.DescribeVpcEndpointServices(&servicesInput)
	if err != nil {
		return nil, err
	}
	for _, service := range services.ServiceDetails {
		for _, tag := range service.Tags {
			if *tag.Key == "api.openshift.com/id" && *tag.Value == clusterID {
				clusterEndpointConnections = append(clusterEndpointConnections, service)
			}
		}
	}
	return clusterEndpointConnections, nil
}

func getLoadBalancers(client aws.Client, clusterID string) ([]elbv2types.LoadBalancer, error) {
	loadbalancerTag := "kubernetes.io/service-name"
	loadbalancers, err := client.DescribeV2LoadBalancers(&elasticloadbalancingv2.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, err
	}
	clusterLoadbalancers := make([]elbv2types.LoadBalancer, 0, len(loadbalancers.LoadBalancers))
	loadBalancerArns := make([]string, 0, len(loadbalancers.LoadBalancers))
	for _, lb := range loadbalancers.LoadBalancers {
		if lb.LoadBalancerArn != nil {
			loadBalancerArns = append(loadBalancerArns, *lb.LoadBalancerArn)
		}
	}
	chunkSize := 20
	var chunks [][]string
	for i := 0; i < len(loadBalancerArns); i += chunkSize {
		end := i + chunkSize
		if end > len(loadBalancerArns) {
			end = len(loadBalancerArns)
		}
		chunks = append(chunks, loadBalancerArns[i:end])
	}
	var tagsOutput []*elasticloadbalancingv2.DescribeTagsOutput
	for _, chunk := range chunks {
		tagsInput := &elasticloadbalancingv2.DescribeTagsInput{
			ResourceArns: chunk,
		}
		loadBalancerTags, err := client.DescribeV2Tags(tagsInput)
		if err != nil {
			return nil, err
		}
		tagsOutput = append(tagsOutput, loadBalancerTags)
	}
	// Can only retrieve *20* resources at a time, so this must be done in chunks:
	lbforCluster := make([]*string, 0, len(loadbalancers.LoadBalancers))
	for _, tagOutput := range tagsOutput {
		for _, lbTag := range tagOutput.TagDescriptions {
			for _, tag := range lbTag.Tags {
				if *tag.Key == loadbalancerTag && strings.Contains(*tag.Value, clusterID) {
					lbforCluster = append(lbforCluster, lbTag.ResourceArn)
				}
			}
		}
	}
	for _, lb := range loadbalancers.LoadBalancers {
		for _, arg := range lbforCluster {
			if *lb.LoadBalancerArn == *arg {
				clusterLoadbalancers = append(clusterLoadbalancers, lb)
			}
		}
	}
	return clusterLoadbalancers, nil
}

func gatherManagementClusterInfo(client aws.Client, clusterID string, c chan ChanReturn[managementClusterInfo]) {
	verboseLog("Fetching resources for Management Cluster")
	// Get the VPC endpoints
	clusterEndpointServices, err := getVpcEndpointServices(client, clusterID)
	if err != nil {
		verboseLog("Could not find matching endpoint services in the management cluster - this is likely a problem.")
		c <- ChanReturn[managementClusterInfo]{
			Value: managementClusterInfo{},
			Error: err,
		}
	}
	if len(clusterEndpointServices) == 0 {
		verboseLog("Could not find matching endpoint services in the management cluster - this is likely a problem.")
	}
	clusterEndpoints, err := getVpcEndpointConnections(client, clusterEndpointServices)
	if err != nil {
		verboseLog("Could not find matching endpoints in the management cluster - this is likely a problem.")
		c <- ChanReturn[managementClusterInfo]{
			Value: managementClusterInfo{},
			Error: err,
		}
	}
	if len(clusterEndpointServices) == 0 {
		verboseLog("Could not find matching endpoints in the management cluster - this is likely a problem.")
	}
	clusterLoadbalancers, err := getLoadBalancers(client, clusterID)
	if err != nil {
		verboseLog("Could not find matching loadbalancers in the management cluster - this is likely a problem.")
		c <- ChanReturn[managementClusterInfo]{
			Value: managementClusterInfo{},
			Error: err,
		}
	}
	if len(clusterEndpointServices) == 0 {
		verboseLog("Could not find matching loadbalancers in the management cluster - this is likely a problem.")
	}
	c <- ChanReturn[managementClusterInfo]{
		Value: managementClusterInfo{
			EndpointConnections: clusterEndpoints,
			EndpointServices:    clusterEndpointServices,
			LoadBalancers:       clusterLoadbalancers,
		},
		Error: nil,
	}
}

func gatherPrivatelinkClusterInfo(client aws.Client, cluster *v1.Cluster, c chan ChanReturn[privatelinkInfo]) {
	clusterID := cluster.ID()
	apiUrl := baseApiUrl(cluster)
	verboseLog("Fetching resources for Privatelink")
	// Get Route53 information
	clusterHostedZones, err := getHostedZones(client, apiUrl)
	if err != nil {
		verboseLog("Could not find matching hosted zones in the privatelink cluster - this is likely a problem.")
		c <- ChanReturn[privatelinkInfo]{
			Value: privatelinkInfo{},
			Error: err,
		}
	}
	if len(clusterHostedZones) == 0 {
		verboseLog("Could not find matching hosted zones in the privatelink cluster - this is likely a problem.")
	}
	// Get ResourceSets in the HostedZones
	rrs, err := getResourceRecordSets(client, clusterHostedZones)
	if err != nil {
		verboseLog("Could not find matching resource records in the privatelink cluster - this is likely a problem.")
		c <- ChanReturn[privatelinkInfo]{
			Value: privatelinkInfo{},
			Error: err,
		}
	}
	if len(rrs) == 0 {
		verboseLog("Could not find matching resource records in the privatelink cluster - this is likely a problem.")
	}
	// Get the VPC endpoints
	clusterEndpoints, err := getVpcEndpoints(client, clusterID, "Name", fmt.Sprintf("%s-private-hcp-vpce", clusterID))
	if err != nil {
		verboseLog("Could not find matching cluster endpoints in the privatelink cluster - this is likely a problem.")
		c <- ChanReturn[privatelinkInfo]{
			Value: privatelinkInfo{},
			Error: err,
		}
	}
	c <- ChanReturn[privatelinkInfo]{
		Value: privatelinkInfo{
			HostedZones:     clusterHostedZones,
			ResourceRecords: rrs,
			Endpoints:       clusterEndpoints,
		},
		Error: err,
	}
}

func getSubnets(client aws.Client, subnetIds []string) ([]ec2types.Subnet, error) {
	subnetFilter := make([]string, 0, len(subnetIds))
	for _, subnet := range subnetIds {
		subnetFilter = append(subnetFilter, subnet)
	}
	input := ec2.DescribeSubnetsInput{
		SubnetIds: subnetFilter,
	}
	subnets, err := client.DescribeSubnets(&input)
	if err != nil {
		return nil, err
	}
	return subnets.Subnets, err
}

func gatherCustomerClusterInfo(client aws.Client, cluster *v1.Cluster, c chan<- ChanReturn[clusterInfo]) {
	clusterID := cluster.ID()
	apiUrl := baseApiUrl(cluster)
	verboseLog("Fetching resources for Customer Cluster")
	// Get the Route53 HostedZone
	clusterHostedZones, err := getHostedZones(client, apiUrl)
	if err != nil {
		verboseLog("Could not find matching hosted zones in the cluster - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	if len(clusterHostedZones) == 0 {
		verboseLog("Could not find matching hosted zones in the cluster - this is likely a problem.")
	}
	// Get the private HostedZone used to communicate from cluster -> apiserver
	privateZoneName := fmt.Sprintf("%s.hypershift.local", cluster.Name())
	privateHostedZones, err := getHostedZones(client, privateZoneName)
	if err != nil {
		verboseLog("Could not find matching hosted zones in the cluster - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	if len(privateHostedZones) == 0 {
		verboseLog("Could not find matching private hosted zones in the cluster - this is likely a problem.")
	}
	clusterHostedZones = append(clusterHostedZones, privateHostedZones...)
	// Get ResourceSets in the HostedZones
	rrs, err := getResourceRecordSets(client, clusterHostedZones)
	if err != nil {
		verboseLog("Could not find record sets in the cluster - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	if len(clusterHostedZones) == 0 {
		verboseLog("Could not find endpoints in the cluster - this is likely a problem.")
	}
	// Get VPC Endpoints
	clusterEndpoints, err := getVpcEndpoints(client, clusterID, fmt.Sprintf("kubernetes.io/cluster/%s", clusterID), "owned")
	if err != nil {
		verboseLog("Could not find endpoints in the cluster - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	if len(clusterHostedZones) == 0 {
		verboseLog("Could not find endpoints in the cluster - this is likely a problem.")
	}
	// Get Subnets
	clusterSubnets, err := getSubnets(client, cluster.AWS().SubnetIDs())
	if err != nil {
		verboseLog("Could not find subnets in the cluster - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	// Get RouteTables
	clusterRouteTables, err := getRouteTables(client, cluster.AWS().SubnetIDs())
	if err != nil {
		verboseLog("Could not find routetables in the subnets - this is likely a problem.")
		c <- ChanReturn[clusterInfo]{
			Value: clusterInfo{},
			Error: err,
		}
	}
	c <- ChanReturn[clusterInfo]{
		Value: clusterInfo{
			HostedZones:       clusterHostedZones,
			ResourceRecords:   rrs,
			Endpoints:         clusterEndpoints,
			Subnets:           clusterSubnets,
			SubnetRouteTables: clusterRouteTables,
		},
		Error: nil,
	}
}

func getRouteTables(client aws.Client, subnets []string) ([]ec2types.RouteTable, error) {
	routeTables := make([]ec2types.RouteTable, 0)
	filterKey := "association.subnet-id"
	for _, subnet := range subnets {
		input := ec2.DescribeRouteTablesInput{
			Filters: []ec2types.Filter{
				{
					Name:   &filterKey,
					Values: []string{subnet},
				},
			},
		}
		rtbs, err := client.DescribeRouteTables(&input)
		if err != nil {
			return nil, err
		}
		routeTables = append(routeTables, rtbs.RouteTables...)
	}
	return routeTables, nil
}

func render(ainfo *aggregateClusterInfo) {
	// Expected connection that we want to render:
	// - API Resource record set in the privatelink cluster should connect to the VPCE in the MC cluster
	// - VPCE in the MC cluster should connect to the LB
	fmt.Println("[PRIVATELINK]")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Hostedzone ID", "Hostedzone Name", "Private"})
	for _, hz := range ainfo.privatelinkInfo.HostedZones {
		table.Append([]string{*hz.Id, *hz.Name, strconv.FormatBool(hz.Config.PrivateZone)})
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Resource Record", "Target"})
	table.SetAutoMergeCells(true)
	for _, rr := range ainfo.privatelinkInfo.ResourceRecords {
		for _, subr := range rr.ResourceRecords {
			table.Append([]string{*rr.Name, *subr.Value})
		}
	}
	table.Render()

	fmt.Println("[MANAGEMENT CLUSTER]")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"LoadBalancer Name", "DNSName", "LoadBalancerArn", "HostedZoneID"})
	for _, lb := range ainfo.managementClusterInfo.LoadBalancers {
		table.Append([]string{*lb.LoadBalancerName, *lb.DNSName, *lb.LoadBalancerArn, *lb.CanonicalHostedZoneId})
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Service Name", "Service ID", "Service Type", "DNS Name", "Owner"})
	table.SetAutoMergeCells(true)
	for _, svc := range ainfo.managementClusterInfo.EndpointServices {
		for _, dnsname := range svc.BaseEndpointDnsNames {
			servicetype := string(svc.ServiceType[0].ServiceType)
			table.Append([]string{*svc.ServiceName, *svc.ServiceId, servicetype, dnsname, *svc.Owner})
		}
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Connection ID", "Endpoint ID", "Endpoint State", "Endpoint Owner", "AddressType", "DNS", "LB"})
	for _, conn := range ainfo.managementClusterInfo.EndpointConnections {
		for _, dns := range conn.DnsEntries {
			endpointstate := string(conn.VpcEndpointState)
			ipaddresstype := string(conn.IpAddressType)
			table.Append([]string{*conn.VpcEndpointConnectionId, *conn.VpcEndpointId, endpointstate, *conn.VpcEndpointOwner, ipaddresstype, *dns.DnsName, conn.NetworkLoadBalancerArns[0]})
		}
	}
	table.Render()

	fmt.Println("[CUSTOMER CLUSTER]")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Hostedzone ID", "Hostedzone Name", "Private"})
	for _, hz := range ainfo.clusterInfo.HostedZones {
		table.Append([]string{*hz.Id, *hz.Name, strconv.FormatBool(hz.Config.PrivateZone)})
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Resource Record", "Target"})
	table.SetAutoMergeCells(true)
	for _, rr := range ainfo.clusterInfo.ResourceRecords {
		for _, subr := range rr.ResourceRecords {
			table.Append([]string{safeDeref(rr.Name), safeDeref(subr.Value)})
		}
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Endpoint ID", "Type", "VPC", "Service", "State"})
	for _, ep := range ainfo.clusterInfo.Endpoints {
		eptype := string(ep.VpcEndpointType)
		state := string(ep.State)
		table.Append([]string{safeDeref(ep.VpcEndpointId), eptype, safeDeref(ep.VpcId), safeDeref(ep.ServiceName), state})
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"RouteTable", "VPC", "Destination CIDR", "Destination Gateway"})
	table.SetAutoMergeCells(true)
	for _, rtb := range ainfo.clusterInfo.SubnetRouteTables {
		for _, route := range rtb.Routes {
			var destination, targetId string
			if route.DestinationCidrBlock != nil {
				destination = *route.DestinationCidrBlock
			} else {
				destination = *route.DestinationPrefixListId
			}
			if route.GatewayId != nil {
				targetId = *route.GatewayId
			} else if route.NatGatewayId != nil {
				targetId = *route.NatGatewayId
			} else if route.TransitGatewayId != nil {
				targetId = *route.TransitGatewayId
			} else if route.LocalGatewayId != nil {
				targetId = *route.LocalGatewayId
			} else {
				targetId = "Unknown"
			}
			table.Append([]string{safeDeref(rtb.RouteTableId), safeDeref(rtb.VpcId), destination, targetId})
		}
	}
	table.Render()
}

func safeDeref(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func verboseLog(msg ...interface{}) {
	if verbose {
		fmt.Println(msg...)
	}
}
