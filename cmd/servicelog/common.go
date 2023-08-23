package servicelog

import (
	"encoding/json"
	"fmt"
	"github.com/openshift/osdctl/internal/servicelog"
	sl "github.com/openshift/osdctl/internal/servicelog"
	"time"
)

var (
	userParameterNames, userParameterValues, filterParams []string
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
func GetServiceLogsSince(clusterID string, days int) ([]sl.ServiceLogShort, error) {
	// time.Now().Sub() returns the duration between two times, so we negate the duration in Add()
	earliestTime := time.Now().AddDate(0, 0, -days)

	slResponse, err := FetchServiceLogs(clusterID)
	if err != nil {
		return nil, err
	}

	var serviceLogs sl.ServiceLogShortList
	err = json.Unmarshal(slResponse.Bytes(), &serviceLogs)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal the SL response %w", err)

	}

	// Parsing the relevant service logs
	// - We only care about SLs sent in the given duration
	var errorServiceLogs []sl.ServiceLogShort
	for _, serviceLog := range serviceLogs.Items {
		if serviceLog.CreatedAt.After(earliestTime) {
			errorServiceLogs = append(errorServiceLogs, serviceLog)
		}
	}

	return errorServiceLogs, nil
}
