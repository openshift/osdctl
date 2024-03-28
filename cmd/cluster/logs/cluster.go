package logs

import (
	"fmt"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"os"
)

func fetchDetails(clusterKey string) (string, string, string) {
	if err := ocmutils.IsValidClusterKey(clusterKey); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterKey)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	MgmtClusterID, MgmtClusterName, err := determineManagementCluster(connection, cluster)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	url, err := getDynatraceURLFromLabel(connection, MgmtClusterID)
	if err != nil {
		fmt.Printf("the Dynatrace Environemnt URL could not be determined. \nPlease refer the SOP to determine the correct Dyntrace Tenant URL- https://github.com/openshift/ops-sop/tree/master/dynatrace#what-environments-are-there \n\nError Details - %s", err)
		os.Exit(1)
	}
	return cluster.ID(), MgmtClusterName, url
}

func determineManagementCluster(connection *sdk.Connection, cluster *v1.Cluster) (string, string, error) {
	var clusterID = cluster.ID()
	var clusterName = cluster.Name()
	if cluster.Hypershift().Enabled() {
		ManagementCluster, err := ocmutils.GetManagementCluster(clusterID)
		if err != nil {
			return "", "", fmt.Errorf("error retreiving Management Cluster for given HCP")
		}
		clusterID = ManagementCluster.ID()
		clusterName = ManagementCluster.Name()
	}

	if !isManagementCluster(connection, clusterID) && !cluster.Hypershift().Enabled() {
		return "", "", fmt.Errorf("cluster is not a HCP/Management Cluster")
	}
	return clusterID, clusterName, nil
}

// Sanity Check for MC Cluster
func isManagementCluster(connection *sdk.Connection, clusterID string) bool {
	collection := connection.ClustersMgmt().V1().Clusters()
	// Get the labels externally available for the cluster
	resource := collection.Cluster(clusterID).ExternalConfiguration().Labels()
	// Send the request to retrieve the list of external cluster labels:
	response, err := resource.List().Send()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster labels: %v\n", err)
		os.Exit(1)
	}

	labels, ok := response.GetItems()
	if !ok {
		return false
	}

	for _, label := range labels.Slice() {
		if l, ok := label.GetKey(); ok {
			// If the label is found as the key, we know its an Managemnt Cluster
			if l == HypershiftClusterTypeLabel {
				return true
			}
		}
	}
	return false
}
func getDynatraceURLFromLabel(connection *sdk.Connection, clusterID string) (string, error) {
	subscription, err := ocmutils.GetSubscription(connection, clusterID)
	if err != nil {
		return "", err
	}

	subscriptionLabels, err := connection.AccountsMgmt().V1().Subscriptions().Subscription(subscription.ID()).Labels().List().Send()
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
