package servicelog

import (
	"fmt"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	v1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	log "github.com/sirupsen/logrus"
)

// checkServiceLogsLastHour returns true if there were service logs sent
// in the past hour, printing warnings for each one found.
func CheckServiceLogsLastHour(ocmClient *sdk.Connection, clusterID string) bool {
	timeStampToCompare := time.Now().Add(-time.Hour)
	serviceLogs, err := GetServiceLogsSince(ocmClient, clusterID, timeStampToCompare, false, false)
	if err != nil {
		log.Warnf("please verify that you are not sending a duplicate service log that has been recently sent - failed to fetch recent service logs: %v", err)
		return true
	}
	if len(serviceLogs) > 0 {
		for _, svclog := range serviceLogs {
			log.Warnf("A service log has been submitted in last hour\nDescription: %s", svclog.Description())
		}
		return true
	}
	return false
}

// GetServiceLogsSince returns the service logs for a cluster sent since the
// given time.
func GetServiceLogsSince(ocmClient *sdk.Connection, clusterID string, timeSince time.Time, allMessages bool, internalOnly bool) ([]*v1.LogEntry, error) {
	slResponse, err := FetchServiceLogs(ocmClient, clusterID, allMessages, internalOnly)
	if err != nil {
		return nil, err
	}

	var recentLogs []*v1.LogEntry
	for _, serviceLog := range slResponse.Items().Slice() {
		if serviceLog.CreatedAt().After(timeSince) {
			recentLogs = append(recentLogs, serviceLog)
		}
	}

	return recentLogs, nil
}

// FetchServiceLogs retrieves all service logs for a cluster.
func FetchServiceLogs(ocmClient *sdk.Connection, clusterID string, allMessages bool, internalOnly bool) (*v1.ClustersClusterLogsListResponse, error) {
	clusters := ocmutils.GetClusters(ocmClient, []string{clusterID})
	if len(clusters) != 1 {
		return nil, fmt.Errorf("GetClusters expected to return 1 cluster, got: %d", len(clusters))
	}

	return sendClusterLogsListRequest(ocmClient, clusters[0], allMessages, internalOnly)
}

func sendClusterLogsListRequest(ocmClient *sdk.Connection, cluster *cmv1.Cluster, allMessages bool, internalMessages bool) (*v1.ClustersClusterLogsListResponse, error) {
	request := ocmClient.ServiceLogs().V1().Clusters().ClusterLogs().List().
		ClusterID(cluster.ID()).
		ClusterUUID(cluster.ExternalID()).
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
