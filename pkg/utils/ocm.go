package utils

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/google/uuid"
	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

const ClusterServiceClusterSearch = "id = '%s' or name = '%s' or external_id = '%s'"

const (
	productionURL              = "https://api.openshift.com"
	stagingURL                 = "https://api.stage.openshift.com"
	integrationURL             = "https://api.integration.openshift.com"
	productionGovURL           = "https://api-admin.openshiftusgov.com"
	integrationGovURL          = "https://api-admin.int.openshiftusgov.com"
	stageGovURL                = "https://api-admin.stage.openshiftusgov.com"
	HypershiftClusterTypeLabel = "ext-hypershift.openshift.io/cluster-type"
	DynatraceTenantKeyLabel    = "sre-capabilities.dtp.tenant"
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
	"stagegov":        stageGovURL,
	stageGovURL:       stageGovURL,
}

// Config describes the OCM client configuration
// Taken wholesale from openshift-online/ocm-cli
type Config struct {
	AccessToken  string   `json:"access_token,omitempty" doc:"Bearer access token."`
	ClientID     string   `json:"client_id,omitempty" doc:"OpenID client identifier."`
	ClientSecret string   `json:"client_secret,omitempty" doc:"OpenID client secret."`
	Insecure     bool     `json:"insecure,omitempty" doc:"Enables insecure communication with the server. This disables verification of TLS certificates and host names."`
	Password     string   `json:"password,omitempty" doc:"User password."`
	RefreshToken string   `json:"refresh_token,omitempty" doc:"Offline or refresh token."`
	Scopes       []string `json:"scopes,omitempty" doc:"OpenID scope. If this option is used it will replace completely the default scopes. Can be repeated multiple times to specify multiple scopes."`
	TokenURL     string   `json:"token_url,omitempty" doc:"OpenID token URL."`
	URL          string   `json:"url,omitempty" doc:"URL of the API gateway. The value can be the complete URL or an alias. The valid aliases are 'production', 'staging' and 'integration'."`
	User         string   `json:"user,omitempty" doc:"User name."`
	Pager        string   `json:"pager,omitempty" doc:"Pager command, for example 'less'. If empty no pager will be used."`
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
	subID, ok := cluster.Subscription().GetID()
	if !ok {
		return "", fmt.Errorf("failed getting sub id")
	}

	resp, err := ocmClient.AccountsMgmt().V1().Subscriptions().List().Search(fmt.Sprintf("id like '%s'", subID)).Size(1).Send()
	if err != nil {
		return "", err
	}

	respSlice := resp.Items().Slice()
	if len(respSlice) > 1 {
		return "", fmt.Errorf("expected only 1 org to be returned")
	}

	return respSlice[0].OrganizationID(), nil
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

// Finds the OCM Configuration file and returns the path to it
// Taken wholesale from	openshift-online/ocm-cli
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

// Loads the OCM Configuration file
// Taken wholesale from	openshift-online/ocm-cli
func loadOCMConfig() (*Config, error) {
	var err error

	file, err := getOCMConfigLocation()
	if err != nil {
		return nil, err
	}

	_, err = os.Stat(file)
	if os.IsNotExist(err) {
		cfg := &Config{}
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

	cfg := &Config{}
	err = json.Unmarshal(data, cfg)

	if err != nil {
		err = fmt.Errorf("can't parse config file '%s': %v", file, err)
		return cfg, err
	}

	return cfg, nil
}

func getOcmConfiguration(ocmConfigLoader func() (*Config, error)) (*Config, error) {
	tokenEnv := os.Getenv("OCM_TOKEN")
	urlEnv := os.Getenv("OCM_URL")
	refreshTokenEnv := os.Getenv("OCM_REFRESH_TOKEN") // Unlikely to be set, but check anyway

	config := &Config{}

	// If missing required data, load from the config file.
	// We don't want to always load this, because the user might only use environment variables.
	if tokenEnv == "" || refreshTokenEnv == "" || urlEnv == "" {
		var fileConfigLoadError error
		config, fileConfigLoadError = ocmConfigLoader()
		if fileConfigLoadError != nil {
			return config, fmt.Errorf("could not load OCM configuration file")
		}
	}

	// Overwrite with set environment variables, to allow users to overwrite
	// their configuration file's variables
	if tokenEnv != "" {
		config.AccessToken = tokenEnv
	}
	if urlEnv != "" {
		config.URL = urlEnv
	}
	if refreshTokenEnv != "" {
		config.RefreshToken = refreshTokenEnv
	}

	return config, nil
}

func CreateConnection() (*sdk.Connection, error) {
	ocmConfigError := "Unable to load OCM config\nLogin with 'ocm login' or set OCM_TOKEN, OCM_URL and OCM_REFRESH_TOKEN environment variables"

	connectionBuilder := sdk.NewConnectionBuilder()

	config, err := getOcmConfiguration(loadOCMConfig)
	if err != nil {
		return nil, errors.New(ocmConfigError)
	}

	connectionBuilder.Tokens(config.AccessToken, config.RefreshToken)

	if config.URL == "" {
		return nil, errors.New(ocmConfigError)
	}

	// Parse the URL in case it is an alias
	gatewayURL, ok := urlAliases[config.URL]
	if !ok {
		return nil, fmt.Errorf("invalid OCM_URL found: %s\nValid URL aliases are: 'production', 'staging', 'integration'", config.URL)
	}
	connectionBuilder.URL(gatewayURL)

	connectionBuilder.Client(config.ClientID, config.ClientSecret)

	connection, err := connectionBuilder.Build()

	if err != nil {
		if strings.Contains(err.Error(), "Not logged in, run the") {
			return nil, errors.New(ocmConfigError)
		}
		return nil, fmt.Errorf("failed to create OCM connection: %v", err)
	}

	return connection, nil
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
