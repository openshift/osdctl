package servicelog

import (
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/internal/servicelog"
	"github.com/openshift/osdctl/pkg/utils"
)

var (
	userParameterNames, userParameterValues []string
)

const (
	// in case you want to see the swagger code gen, you can look at
	// https://api.openshift.com/?urls.primaryName=Service%20logs#/default/post_api_service_logs_v1_cluster_logs
	targetAPIPath = "/api/service_logs/v1/cluster_logs"
)

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

// GetServiceLogsSince returns the service logs for a cluster sent between
// time.Now() and time.Now()-duration. the first parameter will contain a slice
// of the service logs from the given time period, while the second return value
// indicates if an error has happened.
func GetServiceLogsSince(clusterID string, timeSince time.Time, allMessages bool, internalOnly bool) ([]*v1.LogEntry, error) {
	earliestTime := timeSince

	slResponse, err := FetchServiceLogs(clusterID, allMessages, internalOnly)
	if err != nil {
		return nil, err
	}

	var errorServiceLogs []*v1.LogEntry
	for _, serviceLog := range slResponse.Items().Slice() {
		if serviceLog.CreatedAt().After(earliestTime) {
			errorServiceLogs = append(errorServiceLogs, serviceLog)
		}
	}

	return errorServiceLogs, nil
}

func FetchServiceLogs(clusterID string, allMessages bool, internalOnly bool) (*v1.ClustersClusterLogsListResponse, error) {
	// Create OCM client to talk to cluster API
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Use the OCM client to retrieve clusters
	clusters := utils.GetClusters(ocmClient, []string{clusterID})
	if len(clusters) != 1 {
		return nil, fmt.Errorf("GetClusters expected to return 1 cluster, got: %d", len(clusters))
	}
	cluster := clusters[0]

	// Now get the SLs for the cluster
	clusterLogsListResponse, err := sendClusterLogsListRequest(ocmClient, cluster, allMessages, internalOnly)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service logs for cluster %v: %w", clusterID, err)
	}
	return clusterLogsListResponse, nil
}

func sendClusterLogsListRequest(ocmClient *sdk.Connection, cluster *cmv1.Cluster, allMessages bool, internalMessages bool) (*v1.ClustersClusterLogsListResponse, error) {
	request := ocmClient.ServiceLogs().V1().Clusters().ClusterLogs().List().
		Parameter("cluster_id", cluster.ID()).
		Parameter("cluster_uuid", cluster.ExternalID()).
		Parameter("orderBy", "timestamp desc")

	var searchQuery string
	if !allMessages {
		searchQuery = "service_name='SREManualAction'"
	}
	if internalMessages {
		if searchQuery != "" {
			searchQuery += " and "
		}
		searchQuery += "internal_only='true'"
	}
	request.Search(searchQuery)

	response, err := request.Send()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch service logs: %w", err)
	}
	return response, nil
}
