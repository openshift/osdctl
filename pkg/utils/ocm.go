package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/google/uuid"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amsv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/viper"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ocmConfig "github.com/openshift-online/ocm-common/pkg/ocm/config"
	ocmConnBuilder "github.com/openshift-online/ocm-common/pkg/ocm/connection-builder"
)

const ClusterServiceClusterSearch = "id = '%s' or name = '%s' or external_id = '%s'"

const (
	productionURL              = "https://api.openshift.com"
	stagingURL                 = "https://api.stage.openshift.com"
	integrationURL             = "https://api.integration.openshift.com"
	productionGovURL           = "https://api-admin.openshiftusgov.com"
	integrationGovURL          = "https://api-admin.int.openshiftusgov.com"
	stagingGovURL              = "https://api-admin.stage.openshiftusgov.com"
	HypershiftClusterTypeLabel = "ext-hypershift.openshift.io/cluster-type"
	DynatraceTenantKeyLabel    = "dynatrace.regional-tenant"
)

var urlAliases = map[string]string{
	"production":      productionURL,
	"prod":            productionURL,
	"prd":             productionURL,
	productionURL:     productionURL,
	"staging":         stagingURL,
	"stage":           stagingURL,
	"stg":             stagingURL,
	stagingURL:        stagingURL,
	"integration":     integrationURL,
	"int":             integrationURL,
	integrationURL:    integrationURL,
	"productiongov":   productionGovURL,
	"prodgov":         productionGovURL,
	"prdgov":          productionGovURL,
	productionGovURL:  productionGovURL,
	"integrationgov":  integrationGovURL,
	"intgov":          integrationGovURL,
	integrationGovURL: integrationGovURL,
	"staginggov":      stagingGovURL,
	"stagegov":        stagingGovURL,
	stagingGovURL:     stagingGovURL,
}

// GetClusterAnyStatus returns an OCM cluster object given an OCM connection and cluster id
// (internal id, external id, and name all supported).
func GetClusterAnyStatus(conn *sdk.Connection, clusterId string) (*cmv1.Cluster, error) {
	// identifier in the accounts management service. To find those clusters we need to check
	// directly in the clusters management service.
	clustersSearch := fmt.Sprintf(ClusterServiceClusterSearch, clusterId, clusterId, clusterId)
	clustersListResponse, err := conn.ClustersMgmt().V1().Clusters().List().Search(clustersSearch).Size(1).Send()
	if err != nil {
		return nil, fmt.Errorf("can't retrieve clusters for clusterId '%s': %v", clusterId, err)
	}

	// If there is exactly one cluster matching then return it:
	clustersTotal := clustersListResponse.Total()
	if clustersTotal == 1 {
		return clustersListResponse.Items().Slice()[0], nil
	}

	return nil, fmt.Errorf("there are %d clusters with identifier or name '%s', expected 1", clustersTotal, clusterId)
}

func GetClusters(ocmClient *sdk.Connection, clusterIds []string) []*cmv1.Cluster {
	for i, id := range clusterIds {
		clusterIds[i] = GenerateQuery(id)
	}

	clusters, err := ApplyFilters(ocmClient, []string{strings.Join(clusterIds, " or ")})
	if err != nil {
		log.Fatalf("error while retrieving cluster(s) from ocm: %[1]s", err)
	}

	return clusters
}

func GetOrgfromClusterID(ocmClient *sdk.Connection, cluster cmv1.Cluster) (string, error) {
	sub, err := GetSubFromClusterID(ocmClient, cluster)
	if err != nil {
		return "", err
	}

	return sub.OrganizationID(), nil
}

func GetSubFromClusterID(ocmClient *sdk.Connection, cluster cmv1.Cluster) (*amsv1.Subscription, error) {
	subID, ok := cluster.Subscription().GetID()
	if !ok {
		return nil, fmt.Errorf("failed getting sub id")
	}

	resp, err := ocmClient.AccountsMgmt().V1().Subscriptions().List().Search(fmt.Sprintf("id like '%s'", subID)).Size(1).Send()
	if err != nil {
		return nil, err
	}

	respSlice := resp.Items().Slice()
	if len(respSlice) > 1 {
		return nil, fmt.Errorf("expected only 1 sub to be returned")
	} else if len(respSlice) == 0 {
		return nil, fmt.Errorf("subscription not found")
	}

	return respSlice[0], nil
}

func GetInternalClusterID(ocmClient *sdk.Connection, clusterIdentifier string) (string, error) {
	cluster, err := GetCluster(ocmClient, clusterIdentifier)
	if err != nil {
		return "", fmt.Errorf("failed to get cluster: %w", err)
	}

	return cluster.ID(), nil
}

// ApplyFilters retrieves clusters in OCM which match the filters given
func ApplyFilters(ocmClient *sdk.Connection, filters []string) ([]*cmv1.Cluster, error) {
	if len(filters) < 1 {
		return nil, nil
	}

	for k, v := range filters {
		filters[k] = fmt.Sprintf("(%s)", v)
	}

	requestSize := 50
	full_filters := strings.Join(filters, " and ")

	request := ocmClient.ClustersMgmt().V1().Clusters().List().Search(full_filters).Size(requestSize)
	response, err := request.Send()
	if err != nil {
		return nil, err
	}

	items := response.Items().Slice()
	for response.Size() >= requestSize {
		request.Page(response.Page() + 1)
		response, err = request.Send()
		if err != nil {
			return nil, err
		}
		items = append(items, response.Items().Slice()...)
	}

	return items, err
}

// GenerateQuery returns an OCM search query to retrieve all clusters matching an expression (ie- "foo%")
func GenerateQuery(clusterIdentifier string) string {
	// Based on the format of the clusterIdentifier, we can know what it is, so we can simplify ocm query and make it quicker
	if regexp.MustCompile(`^[0-9a-z]{32}$`).MatchString(clusterIdentifier) {
		return strings.TrimSpace(fmt.Sprintf("(id = '%[1]s')", clusterIdentifier))
	} else if _, err := uuid.Parse(clusterIdentifier); err == nil {
		return strings.TrimSpace(fmt.Sprintf("(external_id = '%[1]s')", clusterIdentifier))
	} else {
		return strings.TrimSpace(fmt.Sprintf("(display_name like '%[1]s')", clusterIdentifier))
	}
}

// Finds the OCM Configuration file and returns the path to it.
// ( Taken wholesale from openshift-online/ocm-cli )
func getOCMConfigLocation() (string, error) {
	if ocmconfig := os.Getenv("OCM_CONFIG"); ocmconfig != "" {
		return ocmconfig, nil
	}

	// Determine home directory to use for the legacy file path
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	path := filepath.Join(home, ".ocm.json")

	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		// Determine standard config directory
		configDir, err := os.UserConfigDir()
		if err != nil {
			return path, err
		}

		// Use standard config directory
		path = filepath.Join(configDir, "/ocm/ocm.json")
	}

	return path, nil
}

// Exported function fetch and return OCM config
func GetOCMConfigFromEnv() (*ocmConfig.Config, error) {
	return loadOCMConfig()
}

// Loads the OCM Configuration file
// Taken wholesale from	openshift-online/ocm-cli
func loadOCMConfig() (*ocmConfig.Config, error) {
	var err error

	file, err := getOCMConfigLocation()
	if err != nil {
		return nil, err
	}

	_, err = os.Stat(file)
	if os.IsNotExist(err) {
		cfg := &ocmConfig.Config{}
		err = nil
		return cfg, err
	}

	if err != nil {
		err = fmt.Errorf("can't check if config file '%s' exists: %v", file, err)
		return nil, err
	}

	data, err := os.ReadFile(file)
	if err != nil {
		err = fmt.Errorf("can't read config file '%s': %v", file, err)
		return nil, err
	}

	if len(data) == 0 {
		return nil, nil
	}

	cfg := &ocmConfig.Config{}
	err = json.Unmarshal(data, cfg)

	if err != nil {
		err = fmt.Errorf("can't parse config file '%s': %v", file, err)
		return cfg, err
	}

	return cfg, nil
}

// Creates a connection to OCM
func CreateConnection() (*sdk.Connection, error) {
	urlEnv := os.Getenv("OCM_URL")
	var ocmApiOverride string
	if urlEnv != "" {
		// if the OCM url is overridden by an env var, use that, but first we need to validate it
		// in the case where it may be an alias
		gatewayURL, ok := urlAliases[urlEnv]
		if !ok {
			return nil, fmt.Errorf("invalid OCM_URL found: %s\nValid URL aliases are: 'production', 'staging', 'integration'", urlEnv)
		}

		ocmApiOverride = gatewayURL
	}

	config, err := ocmConfig.Load()
	if err != nil {
		return nil, fmt.Errorf("unable to load OCM config. %w", err)
	}

	agentString := fmt.Sprintf("osdctl-%s", Version)

	connBuilder := ocmConnBuilder.NewConnection().Config(config).AsAgent(agentString)

	if ocmApiOverride != "" {
		connBuilder.WithApiUrl(ocmApiOverride)
	}

	return connBuilder.Build()
}

// Creates a connection to OCM
func CreateConnectionWithUrl(OcmUrl string) (*sdk.Connection, error) {
	var ocmApiUrl string = OcmUrl
	if len(OcmUrl) <= 0 {
		return nil, fmt.Errorf("CreateConnectionWithUrl provided empty OCM URL")
	}
	// First we need to validate URL in the case where it may be an alias
	ocmApiUrl, ok := urlAliases[OcmUrl]
	if !ok {
		return nil, fmt.Errorf("invalid OCM_URL found: %s\nValid URL aliases are: 'production', 'staging', 'integration'", OcmUrl)
	}
	config, err := ocmConfig.Load()
	if err != nil {
		return nil, fmt.Errorf("unable to load OCM config. %w", err)
	}

	agentString := fmt.Sprintf("osdctl-%s", Version)

	connBuilder := ocmConnBuilder.NewConnection().Config(config).AsAgent(agentString)

	if connBuilder == nil {
		return nil, fmt.Errorf("CreateConnectionWithUrl, ocm connection builder returned nil builder")
	}
	connBuilder.WithApiUrl(ocmApiUrl)

	return connBuilder.Build()
}

func GetSupportRoleArnForCluster(ocmClient *sdk.Connection, clusterID string) (string, error) {

	clusterResponse, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return "", err
	}

	// If the cluster is Hypershift, get the ARN from the cluster response body
	if clusterResponse.Body().Hypershift().Enabled() {
		return clusterResponse.Body().AWS().STS().SupportRoleARN(), nil
	}

	// For non-hypershift, the ARN is in the accountclaim
	liveResponse, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterID).Resources().Live().Get().Send()
	if err != nil {
		return "", err
	}

	respBody := liveResponse.Body().Resources()
	if awsAccountClaim, ok := respBody["aws_account_claim"]; ok {

		var claimJson map[string]interface{}
		json.Unmarshal([]byte(awsAccountClaim), &claimJson)

		if spec, ok := claimJson["spec"]; ok {

			if supportRoleArn, ok := spec.(map[string]interface{})["supportRoleARN"]; ok {
				return supportRoleArn.(string), nil
			}
		}

		return "", fmt.Errorf("unable to get role arn from claim JSON")
	}

	return "", fmt.Errorf("cluster does not have AccountClaim")
}

func GetAWSAccountIdForCluster(ocmClient *sdk.Connection, clusterID string) (string, error) {

	roleArn, err := GetSupportRoleArnForCluster(ocmClient, clusterID)
	if err != nil {
		return "", err
	}

	awsRoleArn, err := arn.Parse(roleArn)
	if err != nil {
		return "", err
	}
	return awsRoleArn.AccountID, nil
}

func IsClusterCCS(ocmClient *sdk.Connection, clusterID string) (bool, error) {

	clusterResponse, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(clusterID).Get().Send()
	if err != nil {
		return false, err
	}

	cluster := clusterResponse.Body()
	if cluster.CCS().Enabled() {
		return true, nil
	}

	return false, nil
}

// Returns the hive shard corresponding to a cluster
// e.g. https://api.<hive_cluster>.byo5.p1.openshiftapps.com:6443
func GetHiveShard(clusterID string) (string, error) {
	connection, err := CreateConnection()
	if err != nil {
		return "", err
	}
	defer connection.Close()

	shardPath, err := connection.ClustersMgmt().V1().Clusters().
		Cluster(clusterID).
		ProvisionShard().
		Get().
		Send()

	if err != nil {
		return "", err
	}

	var shard string

	if shardPath != nil {
		shard = shardPath.Body().HiveConfig().Server()
	}

	if shard == "" {
		return "", fmt.Errorf("Unable to retrieve shard for cluster %s", clusterID)
	}

	return shard, nil
}

// Returns the hive shard corresponding to a cluster using provided OCM connection
// e.g. https://api.<hive_cluster>.byo5.p1.openshiftapps.com:6443
func GetHiveShardWithConn(clusterID string, conn *sdk.Connection) (string, error) {
	if conn == nil {
		return "", fmt.Errorf("nil OCM sdk connection provided to GetHiveShardWithConn()")
	}

	shardPath, err := conn.ClustersMgmt().V1().Clusters().
		Cluster(clusterID).
		ProvisionShard().
		Get().
		Send()

	if err != nil {
		return "", err
	}

	var shard string

	if shardPath != nil {
		shard = shardPath.Body().HiveConfig().Server()
	}

	if shard == "" {
		return "", fmt.Errorf("Unable to retrieve shard for cluster %s", clusterID)
	}

	return shard, nil
}

func GetHiveCluster(clusterId string) (*cmv1.Cluster, error) {
	conn, err := CreateConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	provisionShard, err := conn.ClustersMgmt().V1().Clusters().
		Cluster(clusterId).
		ProvisionShard().
		Get().
		Send()
	if err != nil {
		return nil, err
	}

	hiveApiUrl, ok := provisionShard.Body().HiveConfig().GetServer()
	if !ok {
		return nil, fmt.Errorf("no provision shard url found for %s", clusterId)
	}

	resp, err := conn.ClustersMgmt().V1().Clusters().List().
		Parameter("search", fmt.Sprintf("api.url='%s'", hiveApiUrl)).
		Send()
	if err != nil {
		return nil, err
	}

	if resp.Items().Empty() {
		return nil, fmt.Errorf("failed to find cluster with api.url=%s", hiveApiUrl)
	}

	return resp.Items().Get(0), nil
}

func GetHiveBPClientForCluster(clusterID string, options client.Options, elevationReason string, hiveOCMURL string) (client.Client, error) {
	var hiveOCMConn *sdk.Connection
	var err error
	if len(clusterID) <= 0 {
		return nil, fmt.Errorf("GetHiveBPClientForCluster provided empty target cluster ID")
	}
	if len(hiveOCMURL) <= 0 {
		hiveOCMURL = viper.GetString("hive_ocm_url")
	}
	if len(hiveOCMURL) > 0 {
		hiveOCMConn, err = CreateConnectionWithUrl(hiveOCMURL)
		if err != nil {
			return nil, fmt.Errorf("unable to create hive OCM connection with URL:'%s'. Err: %w", hiveOCMURL, err)
		}
		defer hiveOCMConn.Close()
		hiveCluster, err := GetHiveClusterWithConn(clusterID, nil, hiveOCMConn)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch hive cluster for cluster:'%s', ocmURL:'%s', Err:'%v'", clusterID, hiveOCMURL, err)
		}
		if len(elevationReason) > 0 {
			return k8s.NewAsBackplaneClusterAdminWithConn(hiveCluster.ID(), options, hiveOCMConn, elevationReason)
		}
		return k8s.NewWithConn(hiveCluster.ID(), options, hiveOCMConn)
	} else {
		hiveCluster, err := GetHiveCluster(clusterID)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch hive cluster for cluster:'%s', err:'%v'", clusterID, err)
		}
		if len(elevationReason) > 0 {
			return k8s.NewAsBackplaneClusterAdmin(hiveCluster.ID(), options, elevationReason)
		}
		return k8s.New(hiveCluster.ID(), options)
	}
}

// Fetch Hive Cluster with provided OCM connections.
// In the case that the target cluster(stage, integration, etc does not reside
// in the same OCM env as Hive (prod), separate OCM SDK connections
// can be provided for accessing each. If nil is provided a temporary connection using
// the default OCM env vars will be made.
func GetHiveClusterWithConn(clusterId string, clusterOCM *sdk.Connection, hiveOCM *sdk.Connection) (*cmv1.Cluster, error) {
	var err error
	if clusterOCM == nil {
		clusterOCM, err = CreateConnection()
		if err != nil {
			return nil, err
		}
		// If provided by caller do not close, only close if connection created here.
		defer clusterOCM.Close()
	}
	if hiveOCM == nil {
		hiveOCM = clusterOCM
	}
	provisionShard, err := clusterOCM.ClustersMgmt().V1().Clusters().
		Cluster(clusterId).
		ProvisionShard().
		Get().
		Send()
	if err != nil {
		fmt.Printf("Failed to get provisionShard for cluster:'%s', err:'%v'", clusterId, err)
		return nil, err
	}

	hiveApiUrl, ok := provisionShard.Body().HiveConfig().GetServer()
	if !ok {
		fmt.Printf("No provisionShard found for cluster:'%s'", clusterId)
		return nil, fmt.Errorf("no provision shard url found for %s", clusterId)
	}
	resp, err := hiveOCM.ClustersMgmt().V1().Clusters().List().
		Parameter("search", fmt.Sprintf("api.url='%s'", hiveApiUrl)).
		Send()
	if err != nil {
		fmt.Printf("Error listing clusters with hiveApiUrl:'%s'", hiveApiUrl)
		return nil, err
	}

	if resp.Items().Empty() {
		fmt.Printf("Failed to find hive cluster from hiveApiURL:'%s'", hiveApiUrl)
		return nil, fmt.Errorf("failed to find cluster with api.url=%s", hiveApiUrl)
	}

	return resp.Items().Get(0), nil

}

// GetManagementCluster returns the OCM Cluster object for a provided clusterId
func GetManagementCluster(clusterId string) (*cmv1.Cluster, error) {
	conn, err := CreateConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	hypershiftResp, err := conn.ClustersMgmt().V1().Clusters().
		Cluster(clusterId).
		Hypershift().
		Get().
		Send()
	if err != nil {
		return nil, err
	}

	if mgmtClusterName, ok := hypershiftResp.Body().GetManagementCluster(); ok {
		return GetClusterAnyStatus(conn, mgmtClusterName)
	}

	return nil, fmt.Errorf("no management cluster found for %s", clusterId)
}

// GetServiceCluster returns the hypershift Service Cluster object for a provided HCP clusterId
func GetServiceCluster(clusterId string) (*cmv1.Cluster, error) {
	conn, err := CreateConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	var svcClusterName, mgmtClusterName string

	hypershiftResp, err := conn.ClustersMgmt().V1().Clusters().
		Cluster(clusterId).
		Hypershift().
		Get().
		Send()
	if err != nil {
		return nil, err
	}

	if err == nil && hypershiftResp != nil {
		mgmtClusterName = hypershiftResp.Body().ManagementCluster()
	}

	if mgmtClusterName == "" {
		return nil, fmt.Errorf("failed to lookup management cluster for cluster %s", clusterId)
	}

	// Get the osd_fleet_mgmt reference for the given mgmt_cluster
	ofmResp, err := conn.OSDFleetMgmt().V1().ManagementClusters().
		List().
		Parameter("search", fmt.Sprintf("name='%s'", mgmtClusterName)).
		Send()
	if err != nil {
		return nil, fmt.Errorf("failed to get the fleet manager information for management cluster %s", mgmtClusterName)
	}

	if kind := ofmResp.Items().Get(0).Parent().Kind(); kind == "ServiceCluster" {
		svcClusterName = ofmResp.Items().Get(0).Parent().Name()
	}

	svcCluster, err := GetClusterAnyStatus(conn, svcClusterName)
	if err != nil {
		return nil, err
	}

	return svcCluster, nil
}

// Sanity Check for MC Cluster
func IsManagementCluster(clusterID string) (isMC bool, err error) {
	conn, err := CreateConnection()
	if err != nil {
		return false, err
	}
	defer conn.Close()
	collection := conn.ClustersMgmt().V1().Clusters()
	// Get the labels externally available for the cluster
	resource := collection.Cluster(clusterID).ExternalConfiguration().Labels()
	// Send the request to retrieve the list of external cluster labels:
	response, err := resource.List().Send()
	if err != nil {
		return false, fmt.Errorf("can't retrieve cluster labels: %v", err)
	}

	labels, ok := response.GetItems()
	if !ok {
		return false, nil
	}

	for _, label := range labels.Slice() {
		if l, ok := label.GetKey(); ok {
			// If the label is found as the key, we know its an Managemnt Cluster
			if l == HypershiftClusterTypeLabel {
				return true, nil
			}
		}
	}
	return false, nil
}

func IsHostedCluster(clusterID string) (bool, error) {
	conn, err := CreateConnection()
	if err != nil {
		return false, err
	}
	defer conn.Close()

	cluster := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID)
	res, err := cluster.Get().Send()
	if err != nil {
		return false, err
	}

	return res.Body().Hypershift().Enabled(), nil
}

func GetHCPNamespace(clusterId string) (namespace string, err error) {
	conn, err := CreateConnection()
	if err != nil {
		return "", err
	}
	defer conn.Close()

	hypershiftResp, err := conn.ClustersMgmt().V1().Clusters().
		Cluster(clusterId).
		Hypershift().
		Get().
		Send()
	if err != nil {
		return "", err
	}

	if namespace, ok := hypershiftResp.Body().GetHCPNamespace(); ok {
		return namespace, nil
	}

	return "", fmt.Errorf("no hcp namespace found for %s", clusterId)
}

func GetDynatraceURLFromLabel(clusterID string) (url string, err error) {
	conn, err := CreateConnection()
	if err != nil {
		return "", err
	}
	defer conn.Close()
	subscription, err := GetSubscription(conn, clusterID)
	if err != nil {
		return "", err
	}

	subscriptionLabels, err := conn.AccountsMgmt().V1().Subscriptions().Subscription(subscription.ID()).Labels().List().Send()
	labels, ok := subscriptionLabels.GetItems()
	if !ok {
		return "", err
	}

	for _, label := range labels.Slice() {
		if key, ok := label.GetKey(); ok {
			if key == DynatraceTenantKeyLabel {
				if value, ok := label.GetValue(); ok {
					url := fmt.Sprintf("https://%s.apps.dynatrace.com/", value)
					return url, nil
				}
			}
		}
	}
	return "", fmt.Errorf("DT Tenant Not Found")
}

func SendRequest(request *sdk.Request) (*sdk.Response, error) {
	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}

// Creates an OCM Config object from values read at provided filePath
// utils has a local 'copy' of the config struct
// rather than vendor from "github.com/openshift-online/ocm-cli/pkg/config"
func GetOcmConfigFromFilePath(filePath string) (*ocmConfig.Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		err = fmt.Errorf("can't read config file '%s': %v", filePath, err)
		return nil, err
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("empty config file:'%s'", filePath)
	}
	cfg := &ocmConfig.Config{}
	err = json.Unmarshal(data, cfg)
	if err != nil {
		err = fmt.Errorf("can't parse config file '%s': %v", filePath, err)
		return nil, err
	}
	return cfg, nil
}

func GetOCMSdkConnBuilderFromConfig(ocmCfg *ocmConfig.Config) (*sdk.ConnectionBuilder, error) {
	if ocmCfg == nil {
		return nil, fmt.Errorf("nil OCM config provided to OCMSdkConnBuilderFromConfig()")
	}
	// Can use the sdk.connection builder or alternatively omc cli's connection builder wrappers here.
	// Each returns an ocm-sdk connection builder.
	ocmSdkConnBuilder := sdk.NewConnectionBuilder()
	ocmSdkConnBuilder.URL(ocmCfg.URL)
	ocmSdkConnBuilder.Tokens(ocmCfg.AccessToken, ocmCfg.RefreshToken)
	ocmSdkConnBuilder.Client(ocmCfg.ClientID, ocmCfg.ClientSecret)
	return ocmSdkConnBuilder, nil
}

// Returns an ocmSdkConnBuilder with initial values read from provided configFilePath.
func GetOCMSdkConnBuilderFromFilePath(configFilePath string) (*sdk.ConnectionBuilder, error) {
	// Now get the backplane url and access token from OCM...
	ocmConfig, err := GetOcmConfigFromFilePath(configFilePath)
	if err != nil {
		return nil, err
	}
	return GetOCMSdkConnBuilderFromConfig(ocmConfig)

}

// Returns an OCM SDK connection using values read from provided configFilePath.
func GetOCMSdkConnFromFilePath(configFilePath string) (*sdk.Connection, error) {
	ocmSdkConnBuilder, err := GetOCMSdkConnBuilderFromFilePath(configFilePath)
	if err != nil {
		return nil, err
	}
	ocmSdkConn, err := ocmSdkConnBuilder.Build()

	if err != nil {
		return nil, err
	}
	return ocmSdkConn, nil
}
