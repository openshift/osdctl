package utils

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/openshift-online/ocm-cli/pkg/ocm"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

func GetClusters(ocmClient *sdk.Connection, clusterIds []string) []*v1.Cluster {
	for i, id := range clusterIds {
		clusterIds[i] = GenerateQuery(id)
	}

	clusters, err := ApplyFilters(ocmClient, []string{strings.Join(clusterIds, " or ")})
	if err != nil {
		log.Fatalf("error while retrieving cluster(s) from ocm: %[1]s", err)
	}

	return clusters
}

// ApplyFilters retrieves clusters in OCM which match the filters given
func ApplyFilters(ocmClient *sdk.Connection, filters []string) ([]*v1.Cluster, error) {
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
	return strings.TrimSpace(fmt.Sprintf("(id like '%[1]s' or external_id like '%[1]s' or display_name like '%[1]s')", clusterIdentifier))
}

func CreateConnection() *sdk.Connection {
	connection, err := ocm.NewConnection().Build()
	if err != nil {
		if strings.Contains(err.Error(), "Not logged in, run the") {
			log.Fatalf("Failed to create OCM connection: Authentication error, run the 'ocm login' command first.")
		}
		log.Fatalf("Failed to create OCM connection: %v", err)
	}
	return connection
}

func GetSupportRoleArnForCluster(ocmClient *sdk.Connection, clusterID string) (string, error) {
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
