package resize

import (
	"fmt"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"

	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Resize struct {
	client    client.Client
	hive      client.Client
	hiveAdmin client.Client

	cluster      *cmv1.Cluster
	clusterId    string
	instanceType string
}

func NewCmdResize() *cobra.Command {
	resize := &cobra.Command{
		Use:  "resize",
		Args: cobra.NoArgs,
	}

	resize.AddCommand(
		newCmdResizeInfra(),
	)

	return resize
}

func (r *Resize) New() error {
	scheme := runtime.NewScheme()

	// Register machinev1beta1 for Machines
	if err := machinev1beta1.Install(scheme); err != nil {
		return err
	}

	// Register hivev1 for MachinePools
	if err := hivev1.AddToScheme(scheme); err != nil {
		return err
	}

	if err := corev1.AddToScheme(scheme); err != nil {
		return err
	}

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()
	cluster, err := utils.GetClusterAnyStatus(ocmClient, r.clusterId)
	if err != nil {
		return fmt.Errorf("failed to get OCM cluster info for %s: %s", r.clusterId, err)
	}
	r.cluster = cluster
	r.clusterId = cluster.ID()

	hive, err := utils.GetHiveCluster(cluster.ID())
	if err != nil {
		return err
	}

	c, err := k8s.New(cluster.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	hc, err := k8s.New(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	hac, err := k8s.NewAsBackplaneClusterAdmin(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	r.clusterId = cluster.ID()
	r.client = c
	r.hive = hc
	r.hiveAdmin = hac

	return nil
}
