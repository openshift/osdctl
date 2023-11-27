package cluster

import (
	"context"
	"fmt"
	"net/url"
	"os"

	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1"
	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1/dynakube"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const HypershiftClusterTypeLabel string = "ext-hypershift.openshift.io/cluster-type"

func newCmdDynatraceURL() *cobra.Command {
	orgIdCmd := &cobra.Command{
		Use:   "dynatrace CLUSTER_ID",
		Short: "Get the Dyntrace Tenant URL for a given MC or HCP cluster",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(fetchDetails(args[0]))
		},
	}
	return orgIdCmd
}

func fetchDetails(clusterKey string) error {
	if err := ocmutils.IsValidClusterKey(clusterKey); err != nil {
		return err
	}
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterKey)
	if err != nil {
		return err
	}

	clusterID, err := determineManagementCluster(connection, cluster)
	if err != nil {
		return err
	}

	url, err := GetDynatraceURLFromManagementCluster(clusterID)
	if err != nil {
		return fmt.Errorf("the Dynatrace Environemnt URL could not be determined. \nPlease refer the SOP to determine the correct Dyntrace Tenant URL- https://github.com/openshift/ops-sop/tree/master/dynatrace#what-environments-are-there \n\nError Details - %s", err)
	}
	fmt.Println("Dynatrace Environment URL - ", url)
	return nil
}

func determineManagementCluster(connection *sdk.Connection, cluster *v1.Cluster) (string, error) {
	var clusterID = cluster.ID()
	if cluster.Hypershift().Enabled() {
		ManagementCluster, err := ocmutils.GetManagementCluster(clusterID)
		if err != nil {
			return "", fmt.Errorf("error retreiving Management Cluster for given HCP")
		}
		clusterID = ManagementCluster.ID()
	}

	if !isManagementCluster(connection, clusterID) && !cluster.Hypershift().Enabled() {
		return "", fmt.Errorf("cluster is not a HCP/Management Cluster")
	}
	return clusterID, nil
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
