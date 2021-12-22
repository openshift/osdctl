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

// generateQuery returns an OCM search query to retrieve all clusters matching an expression (ie- "foo%")
func generateQuery(clusterIdentifier string) string {
	return strings.TrimSpace(fmt.Sprintf("(id like '%[1]s' or external_id like '%[1]s' or display_name like '%[1]s')", clusterIdentifier))
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
	full_filters := strings.Join(filters, " and ")

	log.Infof(`running the command: 'ocm list clusters --parameter=search="%s"'`, full_filters)

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

func sendRequest(request *sdk.Request) (*sdk.Response, error) {
	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("cannot send request: %q", err)
	}
	return response, nil
}

func check(response *sdk.Response, clusterMessage servicelog.Message) {
	body := response.Bytes()
	if response.Status() < 400 {
		_, err := validateGoodResponse(body, clusterMessage)
		if err != nil {
			failedClusters[clusterMessage.ClusterUUID] = err.Error()
		} else {
			successfulClusters[clusterMessage.ClusterUUID] = fmt.Sprintf("Message has been successfully sent to %s", clusterMessage.ClusterUUID)
		}
	} else {
		badReply, err := validateBadResponse(body)
		if err != nil {
			failedClusters[clusterMessage.ClusterUUID] = err.Error()
		} else {
			failedClusters[clusterMessage.ClusterUUID] = badReply.Reason
		}
	}
}

func validateGoodResponse(body []byte, clusterMessage servicelog.Message) (goodReply *servicelog.GoodReply, err error) {
	if !json.Valid(body) {
		return nil, fmt.Errorf("server returned invalid JSON")
	}

	if err = json.Unmarshal(body, &goodReply); err != nil {
		return nil, fmt.Errorf("cannot not parse the JSON template.\nError: %q", err)
	}

	if goodReply.Severity != clusterMessage.Severity {
		return nil, fmt.Errorf("message sent, but wrong severity information was passed (wanted %q, got %q)", clusterMessage.Severity, goodReply.Severity)
	}
	if goodReply.ServiceName != clusterMessage.ServiceName {
		return nil, fmt.Errorf("message sent, but wrong service_name information was passed (wanted %q, got %q)", clusterMessage.ServiceName, goodReply.ServiceName)
	}
	if goodReply.ClusterUUID != clusterMessage.ClusterUUID {
		return nil, fmt.Errorf("message sent, but to different cluster (wanted %q, got %q)", clusterMessage.ClusterUUID, goodReply.ClusterUUID)
	}
	if goodReply.Summary != clusterMessage.Summary {
		return nil, fmt.Errorf("message sent, but wrong summary information was passed (wanted %q, got %q)", clusterMessage.Summary, goodReply.Summary)
	}
	if goodReply.Description != clusterMessage.Description {
		return nil, fmt.Errorf("message sent, but wrong description information was passed (wanted %q, got %q)", clusterMessage.Description, goodReply.Description)
	}

	return goodReply, nil
}

func validateBadResponse(body []byte) (badReply *servicelog.BadReply, err error) {
	if ok := json.Valid(body); !ok {
		return nil, fmt.Errorf("server returned invalid JSON")
	}
	if err = json.Unmarshal(body, &badReply); err != nil {
		return nil, fmt.Errorf("cannot parse the error JSON message %q", err)
	}

	return badReply, nil
}
