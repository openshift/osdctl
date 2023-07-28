package health

import (
	"fmt"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"

	hypershift "github.com/openshift/hypershift/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Health struct {
	client                client.Client
	managementClusterName string
	managementCluster     client.Client

	cluster     *cmv1.Cluster
	clusterId   string
	output      string
	verbose     bool
	awsProfile  string
	environment string
}

func NewCmdHealth() *cobra.Command {
	health := &cobra.Command{
		Use:  "health",
		Args: cobra.NoArgs,
	}

	health.AddCommand(
		newCmdClusterHealth(),
	)

	return health
}

func (h *Health) New() error {
	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("Failed to connect to the cluster, %s", err)
	}

	defer ocmClient.Close()
	cluster, err := utils.GetClusterAnyStatus(ocmClient, h.clusterId)
	if err != nil {
		return fmt.Errorf("failed to get OCM cluster info for %s: %s", h.clusterId, err)
	}
	h.cluster = cluster
	h.clusterId = cluster.ID()

	// Register hypershift for Nodepools
	if err := hypershift.AddToScheme(scheme); err != nil {
		return err
	}

	c, err := k8s.New(cluster.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	h.clusterId = cluster.ID()
	h.client = c

	mcName, mcID, err := utils.GetManagementCluster(cluster.ID())
	if err != nil {
		return err
	}

	m, err := k8s.New(mcID, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	h.managementCluster = m
	h.managementClusterName = mcName

	return nil
}
