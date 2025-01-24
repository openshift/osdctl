package dynatrace

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1"
	"github.com/Dynatrace/dynatrace-operator/src/api/v1beta1/dynakube"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/k8s"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
)

type HCPCluster struct {
	name                  string
	internalID            string
	managementClusterID   string
	klusterletNS          string
	hostedNS              string
	hcpNamespace          string
	managementClusterName string
	DynatraceURL          string
	serviceClusterID      string
	serviceClusterName    string
}

var ErrUnsupportedCluster = fmt.Errorf("Not an HCP or MC Cluster")

func FetchClusterDetails(clusterKey string) (hcpCluster HCPCluster, error error) {
	hcpCluster = HCPCluster{}
	if err := ocmutils.IsValidClusterKey(clusterKey); err != nil {
		return hcpCluster, err
	}
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		return HCPCluster{}, err
	}
	defer connection.Close()

	cluster, err := ocmutils.GetCluster(connection, clusterKey)
	if err != nil {
		return HCPCluster{}, err
	}

	if !cluster.Hypershift().Enabled() {
		isMC, err := ocmutils.IsManagementCluster(cluster.ID())
		if !isMC || err != nil {
			// if the cluster is not a HCP or MC, then return an error
			return HCPCluster{}, ErrUnsupportedCluster
		} else {
			// if the cluster is not a HCP but a MC, then return a just relevant info for HCPCluster Object
			hcpCluster.managementClusterID = cluster.ID()
			hcpCluster.managementClusterName = cluster.Name()
			url, err := ocmutils.GetDynatraceURLFromLabel(hcpCluster.managementClusterID)
			if err != nil {
				return HCPCluster{}, fmt.Errorf("the Dynatrace Environemnt URL could not be determined. \nPlease refer the SOP to determine the correct Dyntrace Tenant URL- https://github.com/openshift/ops-sop/tree/master/dynatrace#what-environments-are-there \n\nError Details - %s", err)
			}
			hcpCluster.DynatraceURL = url
			return hcpCluster, nil
		}
	}

	mgmtCluster, err := ocmutils.GetManagementCluster(cluster.ID())
	if err != nil {
		return HCPCluster{}, fmt.Errorf("error retreiving Management Cluster for given HCP %s", err)
	}
	svcCluster, err := ocmutils.GetServiceCluster(cluster.ID())
	if err != nil {
		return HCPCluster{}, fmt.Errorf("error retreiving Service Cluster for given HCP %s", err)
	}
	hcpCluster.hcpNamespace, err = ocmutils.GetHCPNamespace(cluster.ID())
	if err != nil {
		return HCPCluster{}, fmt.Errorf("error retreiving HCP Namespace for given cluster")
	}
	hcpCluster.klusterletNS = fmt.Sprintf("klusterlet-%s", cluster.ID())
	hcpCluster.hostedNS = strings.SplitAfter(hcpCluster.hcpNamespace, cluster.ID())[0]

	url, err := ocmutils.GetDynatraceURLFromLabel(mgmtCluster.ID())
	if err != nil {
		return HCPCluster{}, fmt.Errorf("the Dynatrace Environemnt URL could not be determined. \nPlease refer the SOP to determine the correct Dyntrace Tenant URL- https://github.com/openshift/ops-sop/tree/master/dynatrace#what-environments-are-there \n\nError Details - %s", err)
	}

	hcpCluster.DynatraceURL = url
	hcpCluster.internalID = cluster.ID()
	hcpCluster.managementClusterID = mgmtCluster.ID()
	hcpCluster.name = cluster.Name()
	hcpCluster.managementClusterName = mgmtCluster.Name()
	hcpCluster.serviceClusterID = svcCluster.ID()
	hcpCluster.serviceClusterName = svcCluster.Name()

	return hcpCluster, nil
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
