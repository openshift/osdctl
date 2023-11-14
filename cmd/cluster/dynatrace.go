package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	bplogin "github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var DynatraceURLStaging = map[string]string{
	"us": "https://hrm15629.apps.dynatrace.com/",
	"eu": "https://wmb10400.apps.dynatrace.com/",
}

var DynatraceURLProduction = map[string]string{
	"us": "https://jgn20300.apps.dynatrace.com/",
	"eu": "https://vap62935.apps.dynatrace.com/",
	"ap": "https://zwz85475.apps.dynatrace.com/",
}

func newCmdDynatraceURL() *cobra.Command {
	orgIdCmd := &cobra.Command{
		Use:   "dynatrace CLUSTER_ID",
		Short: "Get the Dyntrace Tenant URL for a given MC cluster",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(fetchDetails(args[0]))
		},
	}
	return orgIdCmd
}

func fetchDetails(clusterID string) error {
	if err := ocmutils.IsValidClusterKey(clusterID); err != nil {
		return err
	}
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterID)
	if err != nil {
		return err
	}
	region := cluster.Region().ID()
	if !strings.HasPrefix(cluster.Name(), "hs-mc-") {
		return fmt.Errorf("cluster is Not a Management Cluster")
	}
	url, err := GetDynatraceURLFromCluster(cluster.ID())
	if err != nil {
		OCMEnv := ocmutils.GetCurrentOCMEnv(connection)
		url = getDynatraceURLFromRegion(region, OCMEnv)
		if url == "" {
			return fmt.Errorf("the Dynatrace Environemnt URL could not be determined")
		}
	}
	fmt.Println("Dynatrace Environment URL - ", url)
	return nil
}

func GetDynatraceURLFromCluster(clusterID string) (string, error) {
	// Login into Cluster
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return "", fmt.Errorf("failed to load backplane-cli config: %v", err)
	}

	kubeconfig, err := bplogin.GetRestConfig(bp, clusterID)
	if err != nil {
		return "", err
	}
	// Create the clientset
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return "", err
	}
	// Fetch Dynakube CRD
	var dynakubeCRD map[string]interface{}
	data, err := clientset.RESTClient().
		Get().
		AbsPath("/apis/dynatrace.com/v1beta1").
		Namespace("dynatrace").
		Resource("dynakubes").
		DoRaw(context.TODO())
	if err != nil {
		return "", err
	}
	err = json.Unmarshal(data, &dynakubeCRD)
	if err != nil {
		return "", err
	}

	// Access the spec.apiUrl field
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return "", err
	}
	var DTUrl string
	for _, item := range result["items"].([]interface{}) {
		apiUrl := item.(map[string]interface{})["spec"].(map[string]interface{})["apiUrl"].(string)
		u, err := url.Parse(apiUrl)
		if err != nil {
			return "", err
		}
		DTUrl = fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	}
	return DTUrl, nil
}

func getDynatraceURLFromRegion(regionID string, OCMEnv string) string {
	regionPrefix := strings.Split(regionID, "-")[0]
	switch OCMEnv {
	case "production":
		return DynatraceURLProduction[regionPrefix]
	case "stage":
		return DynatraceURLStaging[regionPrefix]
	default:
		return ""
	}
}
