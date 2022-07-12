package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

type lmtSprReasonItem struct {
	ID      string
	Summary string
	Details string
}

func GetOCMAccessToken() (*string, error) {
	// Get ocm access token
	ocmCmd := exec.Command("ocm", "token")
	ocmCmd.Stderr = os.Stderr
	ocmOutput, err := ocmCmd.Output()
	if err != nil { // Throw error if ocm not in PATH, or ocm command exit non-zero.
		return nil, fmt.Errorf("failed running ocm token: %v", err)
	}
	accessToken := strings.TrimSuffix(string(ocmOutput), "\n")

	return &accessToken, nil
}

//GetCluster Function allows to get a single cluster with any identifier (displayname, ID, or external ID)
func GetCluster(connection *sdk.Connection, key string) (cluster *cmv1.Cluster, err error) {
	// Prepare the resources that we will be using:
	subsResource := connection.AccountsMgmt().V1().Subscriptions()
	clustersResource := connection.ClustersMgmt().V1().Clusters()

	// Try to find a matching subscription:
	subsSearch := fmt.Sprintf(
		"(display_name = '%s' or cluster_id = '%s' or external_cluster_id = '%s') and "+
			"status in ('Reserved', 'Active')",
		key, key, key,
	)
	subsListResponse, err := subsResource.List().
		Search(subsSearch).
		Size(1).
		Send()
	if err != nil {
		err = fmt.Errorf("Can't retrieve subscription for key '%s': %v", key, err)
		return
	}

	// If there is exactly one matching subscription then return the corresponding cluster:
	subsTotal := subsListResponse.Total()
	if subsTotal == 1 {
		id, ok := subsListResponse.Items().Slice()[0].GetClusterID()
		if ok {
			var clusterGetResponse *cmv1.ClusterGetResponse
			clusterGetResponse, err = clustersResource.Cluster(id).Get().
				Send()
			if err != nil {
				err = fmt.Errorf(
					"Can't retrieve cluster for key '%s': %v",
					key, err,
				)
				return
			}
			cluster = clusterGetResponse.Body()
			return
		}
	}

	// If there are multiple subscriptions that match the cluster then we should report it as
	// an error:
	if subsTotal > 1 {
		err = fmt.Errorf(
			"There are %d subscriptions with cluster identifier or name '%s'",
			subsTotal, key,
		)
		return
	}

	// If we are here then no subscription matches the passed key. It may still be possible that
	// the cluster exists but it is not reporting metrics, so it will not have the external
	// identifier in the accounts management service. To find those clusters we need to check
	// directly in the clusters management service.
	clustersSearch := fmt.Sprintf(
		"id = '%s' or name = '%s' or external_id = '%s'",
		key, key, key,
	)
	clustersListResponse, err := clustersResource.List().
		Search(clustersSearch).
		Size(1).
		Send()
	if err != nil {
		err = fmt.Errorf("Can't retrieve clusters for key '%s': %v", key, err)
		return
	}

	// If there is exactly one cluster matching then return it:
	clustersTotal := clustersListResponse.Total()
	if clustersTotal == 1 {
		cluster = clustersListResponse.Items().Slice()[0]
		return
	}

	// If there are multiple matching clusters then we should report it as an error:
	if clustersTotal > 1 {
		err = fmt.Errorf(
			"There are %d clusters with identifier or name '%s'",
			clustersTotal, key,
		)
		return
	}

	// If we are here then there are no subscriptions or clusters matching the passed key:
	err = fmt.Errorf(
		"There are no subscriptions or clusters with identifier or name '%s'",
		key,
	)
	return
}

func GetClusterLimitedSupportReasons(connection *sdk.Connection, clusterID string) ([]*lmtSprReasonItem, error) {

	limitedSupportReasons, err := connection.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterID).
		LimitedSupportReasons().
		List().
		Send()
	if err != nil {
		return nil, fmt.Errorf("Failed to get limited Support Reasons: %s", err)
	}

	lmtReason := limitedSupportReasons.Items()

	var clusterLmtSprReasons []*lmtSprReasonItem

	lmtReason.Each(func(lmtSprReason *cmv1.LimitedSupportReason) bool {
		clusterLmtSprReason := lmtSprReasonItem{
			ID:      lmtSprReason.ID(),
			Summary: lmtSprReason.Summary(),
			Details: lmtSprReason.Details(),
		}
		clusterLmtSprReasons = append(clusterLmtSprReasons, &clusterLmtSprReason)

		return true
	})

	return clusterLmtSprReasons, nil
}
