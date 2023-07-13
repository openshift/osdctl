package hypershift

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/olekukonko/tablewriter"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const HYPERSHIFT_URL = "/api/clusters_mgmt/v1/clusters/{cluster_id}/hypershift"

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func NewCmdInfo(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newInfoOptions(streams)
	infoCmd := &cobra.Command{
		Use:   "info CLUSTERID",
		Short: "Pull information about AWS objects from the cluster, the management cluster and the privatelink cluster",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("requires a clusterid argument")
			}
			return nil
		},
		ArgAliases:        []string{"clusterid"},
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.clusterID = args[0]
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}
	ops.printFlags.AddFlags(infoCmd)
	infoCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	infoCmd.Flags().StringVarP(&ops.awsRegion, "region", "r", "", "AWS Region")
	infoCmd.Flags().StringVarP(&ops.privatelinkAccountId, "privatelinkaccount", "l", "", "Privatelink account ID")

	return infoCmd
}

// infoOptions defines the struct for running the hypershift info command
type infoOptions struct {
	clusterID            string
	awsProfile           string
	awsRegion            string
	privatelinkAccountId string
	output               string
	printFlags           *printer.PrintFlags
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
	HostedZones     []*route53.HostedZone
	ResourceRecords []*route53.ResourceRecordSet
	Endpoints       []*ec2.VpcEndpoint
}

// Information related to the management cluster
type managementClusterInfo struct {
	EndpointConnections []*ec2.VpcEndpointConnection
	EndpointServices    []*ec2.ServiceDetail
	LoadBalancers       []*elbv2.LoadBalancer
}

// Information related to the privatelink account
type privatelinkInfo struct {
	HostedZones     []*route53.HostedZone
	ResourceRecords []*route53.ResourceRecordSet
	Endpoints       []*ec2.VpcEndpoint
}

// Aggregates all 3 AWS account informations in one
type aggregateClusterInfo struct {
	managementClusterInfo *managementClusterInfo
	clusterInfo           *clusterInfo
	privatelinkInfo       *privatelinkInfo
}

func newInfoOptions(streams genericclioptions.IOStreams) *infoOptions {
	return &infoOptions{
		printFlags: printer.NewPrintFlags(),
		IOStreams:  streams,
	}
}

func (i *infoOptions) complete(cmd *cobra.Command) error {
	var errMsg string
	if i.awsProfile == "" {
		errMsg = "missing argument -p. "
	}
	if i.privatelinkAccountId == "" {
		errMsg += "missing argument -l."
	}
	if errMsg != "" {
		return fmt.Errorf(errMsg)
	}
	return nil
}

func (i *infoOptions) run() error {
	log.Println("Getting hypershift info for cluster: ", i.clusterID)
	clusters, err := i.getClusters()
	if err != nil {
		return err
	}
	if i.awsRegion == "" {
		i.awsRegion = clusters.customerCluster.Region().ID()
		log.Println("Set aws region to: ", i.awsRegion)
	}

	log.Println("Constructing AWS sessions for all accounts")
	awsSessions, err := i.getAWSSessions(clusters)
	if err != nil {
		log.Fatal("Could not construct all AWS sessions: ", err)
		return err
	}

	// To find hostedzones we use the api-url but strip the 'api' and 'clustername' parts
	baseUrlApi := strings.Replace(strings.Replace(strings.Replace(clusters.customerCluster.API().URL(), "https://api.", "", 1), clusters.customerCluster.Name(), "", 1), ":443", "", 1)
	log.Println("Retrieving resources from accounts")
	mcinfo, err := gatherManagementClusterInfo(awsSessions.managementClient, clusters.customerCluster.ID())
	if err != nil {
		log.Fatal(err)
	}
	cinfo, err := gatherCustomerClusterInfo(awsSessions.customerClient, clusters.customerCluster.ID(), baseUrlApi)
	if err != nil {
		log.Fatal(err)
	}
	plinfo, err := gatherPrivatelinkClusterInfo(awsSessions.privatelinkClient, clusters.customerCluster.ID(), baseUrlApi)
	if err != nil {
		log.Fatal(err)
	}
	ai := aggregateClusterInfo{managementClusterInfo: mcinfo,
		clusterInfo:     cinfo,
		privatelinkInfo: plinfo}
	render(ai)
	connections := createGraphViz(ai)
	renderGraphViz(connections)
	return nil
}

func (i *infoOptions) getClusters() (*infoClusters, error) {
	ocmConnection := utils.CreateConnection()
	clusters := utils.GetClusters(ocmConnection, []string{i.clusterID})
	if len(clusters) != 1 {
		errMsg := fmt.Sprint("Did not find cluster with id ", i.clusterID)
		log.Fatal(errMsg)
		return nil, errors.New(errMsg)
	}
	customerCluster := clusters[0]
	if !customerCluster.Hypershift().Enabled() {
		errMsg := fmt.Sprint("Cluster is not a hypershift cluster")
		log.Fatal(errMsg)
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
	json.Unmarshal(response.Bytes(), &hrsp)

	clusters = utils.GetClusters(ocmConnection, []string{hrsp.ManagementCluster})
	if len(clusters) != 1 {
		errMsg := fmt.Sprint("Did not find management cluster with id ", hrsp.ManagementCluster)
		log.Fatal(errMsg)
		return nil, errors.New(errMsg)
	}
	managementCluster := clusters[0]
	return &infoClusters{
		managementCluster: managementCluster,
		customerCluster:   customerCluster,
	}, nil
}

func (i *infoOptions) getAWSSessions(clusters *infoClusters) (*hypershiftAWSClients, error) {
	customerClient, err := osdCloud.GenerateAWSClientForCluster(i.awsProfile, clusters.customerCluster.ID())
	if err != nil {
		return nil, err
	}
	managementClient, err := osdCloud.GenerateAWSClientForCluster(i.awsProfile, clusters.managementCluster.ID())
	if err != nil {
		return nil, err
	}
	awsClient, err := aws.NewAwsClient(i.awsProfile, i.awsRegion, "")
	if err != nil {
		fmt.Printf("Could not build AWS Client: %s\n", err)
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
		fmt.Printf("Could not generate Session Name: %s\n", err)
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
		fmt.Printf("Could not build AWS Client for OrganizationAccountAccessRole: %s\n", err)
		return nil, err
	}

	privatelinkClient, err := aws.NewAwsClientWithInput(&aws.AwsClientInput{
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

func getHostedZones(client aws.Client, apiUrl string) ([]*route53.HostedZone, error) {
	clusterHostedZones := make([]*route53.HostedZone, 0, 1)
	hostedZones, err := client.ListHostedZones(&route53.ListHostedZonesInput{})
	if err != nil {
		return nil, err
	}
	for _, hz := range hostedZones.HostedZones {
		if strings.Contains(*hz.Name, apiUrl) {
			clusterHostedZones = append(clusterHostedZones, hz)
		}
	}
	return clusterHostedZones, nil
}

func getResourceRecordSets(client aws.Client, clusterHostedZones []*route53.HostedZone) ([]*route53.ResourceRecordSet, error) {
	var rrs []*route53.ResourceRecordSet
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

func getVpcEndpointConnections(client aws.Client, services []*ec2.ServiceDetail) ([]*ec2.VpcEndpointConnection, error) {
	serviceIds := make([]*string, 0, len(services))
	for _, service := range services {
		serviceIds = append(serviceIds, service.ServiceId)
	}
	key := "service-id"
	input := ec2.DescribeVpcEndpointConnectionsInput{
		Filters: []*ec2.Filter{
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

func getVpcEndpoints(client aws.Client, clusterID, t, v string) ([]*ec2.VpcEndpoint, error) {
	endpoints, err := client.DescribeVpcEndpoints(&ec2.DescribeVpcEndpointsInput{})
	if err != nil {
		return nil, err
	}
	clusterEndpoints := make([]*ec2.VpcEndpoint, 0)
	for _, endpoint := range endpoints.VpcEndpoints {
		for _, tag := range endpoint.Tags {
			if *tag.Key == t && *tag.Value == v {
				clusterEndpoints = append(clusterEndpoints, endpoint)
			}
		}
	}
	return clusterEndpoints, nil
}

func getVpcEndpointServices(client aws.Client, clusterID string) ([]*ec2.ServiceDetail, error) {
	clusterEndpointConnections := make([]*ec2.ServiceDetail, 0, 1)
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

func getLoadBalancers(client aws.Client, clusterID string) ([]*elbv2.LoadBalancer, error) {
	loadbalancerTag := "kubernetes.io/service-name"
	loadbalancers, err := client.DescribeV2LoadBalancers(&elbv2.DescribeLoadBalancersInput{})
	if err != nil {
		return nil, err
	}
	clusterLoadbalancers := make([]*elbv2.LoadBalancer, 0, len(loadbalancers.LoadBalancers))
	loadBalancerArns := make([]*string, 0, len(loadbalancers.LoadBalancers))
	for _, lb := range loadbalancers.LoadBalancers {
		if lb.LoadBalancerArn != nil {
			loadBalancerArns = append(loadBalancerArns, lb.LoadBalancerArn)
		}
	}
	chunkSize := 20
	var chunks [][]*string
	for i := 0; i < len(loadBalancerArns); i += chunkSize {
		end := i + chunkSize
		if end > len(loadBalancerArns) {
			end = len(loadBalancerArns)
		}
		chunks = append(chunks, loadBalancerArns[i:end])
	}
	var tagsOutput []*elbv2.DescribeTagsOutput
	for _, chunk := range chunks {
		tagsInput := &elbv2.DescribeTagsInput{
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
					log.Println("Found matching loadbalancer for cluster: ", *lbTag.ResourceArn)
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

func gatherManagementClusterInfo(client aws.Client, clusterID string) (*managementClusterInfo, error) {
	log.Println("==> Management Cluster <==")
	// Get the VPC endpoints
	clusterEndpointServices, err := getVpcEndpointServices(client, clusterID)
	if err != nil {
		log.Println("Could not find matching endpoint services in the management cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterEndpointServices) == 0 {
		log.Println("Could not find matching endpoint services in the management cluster - this is likely a problem.")
	}
	clusterEndpoints, err := getVpcEndpointConnections(client, clusterEndpointServices)
	if err != nil {
		log.Println("Could not find matching endpoints in the management cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterEndpointServices) == 0 {
		log.Println("Could not find matching endpoints in the management cluster - this is likely a problem.")
	}
	clusterLoadbalancers, err := getLoadBalancers(client, clusterID)
	if err != nil {
		log.Println("Could not find matching loadbalancers in the management cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterEndpointServices) == 0 {
		log.Println("Could not find matching loadbalancers in the management cluster - this is likely a problem.")
	}
	// Get the loadbalancer
	return &managementClusterInfo{
		EndpointConnections: clusterEndpoints,
		EndpointServices:    clusterEndpointServices,
		LoadBalancers:       clusterLoadbalancers,
	}, err
}

func gatherPrivatelinkClusterInfo(client aws.Client, clusterID string, apiUrl string) (*privatelinkInfo, error) {
	log.Println("==> Privatelink Cluster <==")
	// Get Route53 information
	clusterHostedZones, err := getHostedZones(client, apiUrl)
	if err != nil {
		log.Println("Could not find matching hosted zones in the privatelink cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterHostedZones) == 0 {
		log.Println("Could not find matching hosted zones in the privatelink cluster - this is likely a problem.")
	}
	// Get ResourceSets in the HostedZones
	rrs, err := getResourceRecordSets(client, clusterHostedZones)
	if err != nil {
		log.Println("Could not find matching resource records in the privatelink cluster - this is likely a problem.")
		return nil, err
	}
	if len(rrs) == 0 {
		log.Println("Could not find matching resource records in the privatelink cluster - this is likely a problem.")
	}
	// Get the VPC endpoints
	clusterEndpoints, err := getVpcEndpoints(client, clusterID, "Name", fmt.Sprintf("%s-private-hcp-vpce", clusterID))
	if err != nil {
		log.Println("Could not find matching cluster endpoints in the privatelink cluster - this is likely a problem.")
		return nil, err
	}
	return &privatelinkInfo{
		HostedZones:     clusterHostedZones,
		ResourceRecords: rrs,
		Endpoints:       clusterEndpoints,
	}, nil
}

func gatherCustomerClusterInfo(client aws.Client, clusterID string, apiUrl string) (*clusterInfo, error) {
	log.Println("==> Customer Cluster <==")
	// Get the Route53 HostedZone
	clusterHostedZones, err := getHostedZones(client, apiUrl)
	if err != nil {
		log.Println("Could not find matching hosted zones in the cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterHostedZones) == 0 {
		log.Println("Could not find matching hosted zones in the cluster - this is likely a problem.")
	}
	// Get ResourceSets in the HostedZones
	rrs, err := getResourceRecordSets(client, clusterHostedZones)
	if err != nil {
		log.Println("Could not find record sets in the cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterHostedZones) == 0 {
		log.Println("Could not find endpoints in the cluster - this is likely a problem.")
	}
	// Get VPC Endpoints
	clusterEndpoints, err := getVpcEndpoints(client, clusterID, fmt.Sprintf("kubernetes.io/cluster/%s", clusterID), "owned")
	if err != nil {
		log.Println("Could not find endpoints in the cluster - this is likely a problem.")
		return nil, err
	}
	if len(clusterHostedZones) == 0 {
		log.Println("Could not find endpoints in the cluster - this is likely a problem.")
	}
	// Get the VPC endpoints
	return &clusterInfo{
		HostedZones:     clusterHostedZones,
		ResourceRecords: rrs,
		Endpoints:       clusterEndpoints,
	}, nil
}

func render(ainfo aggregateClusterInfo) {
	// Expected connection that we want to render:
	// - API Resource record set in the privatelink cluster should connect to the VPCE in the MC cluster
	// - VPCE in the MC cluster should connect to the LB
	log.Println("PRIVATELINK")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Hostedzone ID", "Hostedzone Name", "Private"})
	for _, hz := range ainfo.privatelinkInfo.HostedZones {
		table.Append([]string{*hz.Id, *hz.Name, strconv.FormatBool(*hz.Config.PrivateZone)})
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

	log.Println("MANAGEMENT CLUSTER")
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
			table.Append([]string{*svc.ServiceName, *svc.ServiceId, *svc.ServiceType[0].ServiceType, *dnsname, *svc.Owner})
		}
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Connection ID", "Endpoint ID", "Endpoint State", "Endpoint Owner", "AddressType", "DNS", "LB"})
	for _, conn := range ainfo.managementClusterInfo.EndpointConnections {
		for _, dns := range conn.DnsEntries {
			table.Append([]string{*conn.VpcEndpointConnectionId, *conn.VpcEndpointId, *conn.VpcEndpointState, *conn.VpcEndpointOwner, *conn.IpAddressType, *dns.DnsName, *conn.NetworkLoadBalancerArns[0]})
		}
	}
	table.Render()

	log.Println("CUSTOMER CLUSTER")
	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Hostedzone ID", "Hostedzone Name", "Private"})
	for _, hz := range ainfo.clusterInfo.HostedZones {
		table.Append([]string{*hz.Id, *hz.Name, strconv.FormatBool(*hz.Config.PrivateZone)})
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Resource Record", "Target"})
	table.SetAutoMergeCells(true)
	for _, rr := range ainfo.clusterInfo.ResourceRecords {
		for _, subr := range rr.ResourceRecords {
			table.Append([]string{*rr.Name, *subr.Value})
		}
	}
	table.Render()

	table = tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Endpoint ID", "Type", "VPC", "Service", "State"})
	for _, ep := range ainfo.clusterInfo.Endpoints {
		table.Append([]string{"EndpointID", *ep.VpcEndpointType, *ep.VpcEndpointType, *ep.VpcId, *ep.ServiceName, *ep.State})
	}
}
