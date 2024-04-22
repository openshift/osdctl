package dynatrace

import (
	"context"
	"fmt"
	"net/url"

	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1"
	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1/dynakube"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func fetchClusterDetails(clusterKey string) (clusterID string, mcName string, dynatraceURL string, error error) {
	if err := ocmutils.IsValidClusterKey(clusterKey); err != nil {
		return "", "", "", err
	}
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return "", "", "", err
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterKey)
	if err != nil {
		return "", "", "", err
	}

	mgmtClusterID, mgmtClusterName, err := GetManagementCluster(connection, cluster)
	if err != nil {
		return "", "", "", err
	}

	url, err := GetDynatraceURLFromLabel(connection, mgmtClusterID)
	if err != nil {
		return "", "", "", fmt.Errorf("the Dynatrace Environemnt URL could not be determined. \nPlease refer the SOP to determine the correct Dyntrace Tenant URL- https://github.com/openshift/ops-sop/tree/master/dynatrace#what-environments-are-there \n\nError Details - %s", err)
	}
	return cluster.ID(), mgmtClusterName, url, nil
}

func GetManagementCluster(connection *sdk.Connection, cluster *v1.Cluster) (id string, name string, error error) {
	clusterID := cluster.ID()
	clusterName := cluster.Name()
	if cluster.Hypershift().Enabled() {
		ManagementCluster, err := ocmutils.GetManagementCluster(clusterID)
		if err != nil {
			return "", "", fmt.Errorf("error retreiving Management Cluster for given HCP")
		}
		clusterID = ManagementCluster.ID()
		clusterName = ManagementCluster.Name()
	}

	isMC, err := isManagementCluster(connection, clusterID)
	if err != nil {
		return "", "", fmt.Errorf("could not verify if the cluster is HCP/Management Cluster %v", err)
	}

	if !isMC && !cluster.Hypershift().Enabled() {
		return "", "", fmt.Errorf("cluster is not a HCP/Management Cluster")
	}
	return clusterID, clusterName, nil
}

// Sanity Check for MC Cluster
func isManagementCluster(connection *sdk.Connection, clusterID string) (isMC bool, err error) {
	collection := connection.ClustersMgmt().V1().Clusters()
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

func GetDynatraceURLFromLabel(connection *sdk.Connection, clusterID string) (url string, err error) {
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

func GetDynatraceURLFromManagementCluster(clusterID string) (string, error) {
	// Register v1beta1 for DynaKube
	scheme := runtime.NewScheme()
	if err := v1beta1.AddToScheme(scheme); err != nil {
		return "", err
	}
	c, err := k8s.New(clusterID, client.Options{Scheme: scheme})
	if err != nil {
		return "", err
	}

	// Fetch the DynaKube Resource
	var dynaKubeList dynakube.DynaKubeList
	err = c.List(context.TODO(),
		&dynaKubeList,
		&client.ListOptions{Namespace: "dynatrace"})
	if err != nil {
		return "", err
	}
	if len(dynaKubeList.Items) == 0 {
		return "", fmt.Errorf("could not locate dynaKube resource on the cluster")
	}

	// Parse the DT URL
	DTApiURL, err := url.Parse(dynaKubeList.Items[0].Spec.APIURL)
	if err != nil {
		return "", err
	}
	DTURL := fmt.Sprintf("%s://%s", DTApiURL.Scheme, DTApiURL.Host)
	return DTURL, nil
}
