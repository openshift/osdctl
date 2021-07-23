package servicelog

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/ocm"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	log "github.com/sirupsen/logrus"
)

var (
	templateParams, userParameterNames, userParameterValues, filterParams []string
	HTMLBody                                                              []byte
)

const (
	// in case you want to see the swagger code gen, you can look at
	// https://api.openshift.com/?urls.primaryName=Service%20logs#/default/post_api_service_logs_v1_cluster_logs
	targetAPIPath = "/api/service_logs/v1/cluster_logs"
)

func createConnection() *sdk.Connection {
	connection, err := ocm.NewConnection().Build()
	if err != nil {
		if strings.Contains(err.Error(), "Not logged in, run the") {
			log.Fatalf("Failed to create OCM connection: Authetication error, run the 'ocm login' command first.")
		}
		log.Fatalf("Failed to create OCM connection: %v", err)
	}
	return connection
}

// generateClusterQuery returns an OCM search query to identify a cluster
func generateQuery(clusterIdentifier string) string {
	return "id is '" + clusterIdentifier + "' or external_id is '" + clusterIdentifier + "' or display_name i  s '" + clusterIdentifier + "'"
}

// getFilteredClusters retrieves clusters in OCM which match the filters given
func applyFilters(ocmClient *sdk.Connection, filters []string) ([]*v1.Cluster, error) {
	if len(filters) < 1 {
		return nil, nil
	}

	for k, v := range filters {
		filters[k] = fmt.Sprintf("(%s)", v)
	}

	requestSize := 50
	request := ocmClient.ClustersMgmt().V1().Clusters().List().Search(strings.Join(filters, " and ")).Size(requestSize)
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

func sendRequest(request *sdk.Request) *sdk.Response {
	response, err := request.Send()
	if err != nil {
		log.Fatalf("Can't send request: %v", err)
	}
	return response
}

func check(response *sdk.Response, clusterMessage servicelog.Message) {
	body := response.Bytes()
	if response.Status() < 400 {
		validateGoodResponse(body, clusterMessage)
		log.Infof("Message has been successfully sent to %s\n", clusterMessage.ClusterUUID)
	} else {
		badReply := validateBadResponse(body)
		log.Fatalf("Failed to post message because of %q", badReply.Reason)
	}
}

func validateGoodResponse(body []byte, clusterMessage servicelog.Message) servicelog.GoodReply {
	var goodReply servicelog.GoodReply
	if err := json.Unmarshal(body, &goodReply); err != nil {
		log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
	}

	if goodReply.Severity != clusterMessage.Severity {
		log.Fatalf("Message sent, but wrong severity information was passed (wanted %q, got %q)", clusterMessage.Severity, goodReply.Severity)
	}
	if goodReply.ServiceName != clusterMessage.ServiceName {
		log.Fatalf("Message sent, but wrong service_name information was passed (wanted %q, got %q)", clusterMessage.ServiceName, goodReply.ServiceName)
	}
	if goodReply.ClusterUUID != clusterMessage.ClusterUUID {
		log.Fatalf("Message sent, but to different cluster (wanted %q, got %q)", clusterMessage.ClusterUUID, goodReply.ClusterUUID)
	}
	if goodReply.Summary != clusterMessage.Summary {
		log.Fatalf("Message sent, but wrong summary information was passed (wanted %q, got %q)", clusterMessage.Summary, goodReply.Summary)
	}
	if goodReply.Description != clusterMessage.Description {
		log.Fatalf("Message sent, but wrong description information was passed (wanted %q, got %q)", clusterMessage.Description, goodReply.Description)
	}
	if !json.Valid(body) {
		log.Fatalf("Server returned invalid JSON")
	}

	return goodReply
}

func validateBadResponse(body []byte) servicelog.BadReply {
	if ok := json.Valid(body); !ok {
		log.Errorf("Server returned invalid JSON")
	}

	var badReply servicelog.BadReply
	if err := json.Unmarshal(body, &badReply); err != nil {
		log.Fatalf("Cannot parse the error JSON message %q", err)
	}

	return badReply
}
