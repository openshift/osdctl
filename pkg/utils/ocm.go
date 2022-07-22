package utils

import (
	"fmt"
	"log"
	"strings"

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
