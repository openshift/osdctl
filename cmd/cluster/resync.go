package cluster

import (
	"context"
	"fmt"
	"log"
	"time"

	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	twentyMinuteTimeout   = 20 * time.Minute
	twentySecondIncrement = 20 * time.Second
)

type Resync struct {
	hive      client.Client
	clusterId string
}

func newCmdResync() *cobra.Command {
	r := Resync{}

	resyncCmd := &cobra.Command{
		Use:   "resync",
		Short: "Force a resync of a cluster from Hive",
		Long: `Force a resync of a cluster from Hive

  Normally, clusters are periodically synced by Hive every two hours at minimum. This command deletes a cluster's
  clustersync from its managing Hive cluster, causing the clustersync to be recreated in most circumstances and forcing
  a resync of all SyncSets and SelectorSyncSets. The command will also wait for the clustersync to report its status
  again (Success or Failure) before exiting.
`,
		Example: `
  # Force a cluster resync by deleting its clustersync CustomResource
  osdctl cluster resync --cluster-id ${CLUSTER_ID}
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.Run(context.Background())
		},
	}

	resyncCmd.Flags().StringVarP(&r.clusterId, "cluster-id", "C", "", "OCM internal/external cluster id or cluster name to delete the clustersync for.")

	return resyncCmd
}

func (r *Resync) New() error {
	scheme := runtime.NewScheme()

	// Register hiveinternalv1alpha1 for ClusterSync
	if err := hiveinternalv1alpha1.AddToScheme(scheme); err != nil {
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
		return fmt.Errorf("failed to get OCM cluster info for %s: %v", r.clusterId, err)
	}
	r.clusterId = cluster.ID()

	hive, err := utils.GetHiveCluster(cluster.ID())
	if err != nil {
		return err
	}

	hc, err := k8s.New(hive.ID(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	r.hive = hc
	log.Printf("ready to delete clustersync for cluster: %s/%s on hive: %s", cluster.ID(), cluster.Name(), hive.Name())

	return nil
}

func (r *Resync) Run(ctx context.Context) error {
	if err := r.New(); err != nil {
		return fmt.Errorf("failed to initialize command: %v", err)
	}

	ns := &corev1.NamespaceList{}
	selector, err := labels.Parse(fmt.Sprintf("api.openshift.com/id=%s", r.clusterId))
	if err != nil {
		return err
	}

	if err := r.hive.List(ctx, ns, &client.ListOptions{LabelSelector: selector, Limit: 1}); err != nil {
		return err
	}
	if len(ns.Items) != 1 {
		return fmt.Errorf("expected 1 namespace, found %d namespaces with tag: api.openshift.com/id=%s", len(ns.Items), r.clusterId)
	}

	log.Printf("found namespace: %s", ns.Items[0].Name)
	clustersyncs := &hiveinternalv1alpha1.ClusterSyncList{}
	if err := r.hive.List(ctx, clustersyncs, &client.ListOptions{Namespace: ns.Items[0].Name}); err != nil {
		return err
	}
	if len(clustersyncs.Items) != 1 {
		return fmt.Errorf("expected 1 clustersync, found %d clustersyncs in namespace: %s", len(clustersyncs.Items), ns.Items[0].Name)
	}

	log.Printf("deleting clustersync: %s/%s", clustersyncs.Items[0].Namespace, clustersyncs.Items[0].Name)
	if err := r.hive.Delete(ctx, &clustersyncs.Items[0]); err != nil {
		return err
	}

	log.Printf("waiting up to %s for clustersync to report status", twentyMinuteTimeout)
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		clustersync := &hiveinternalv1alpha1.ClusterSync{}
		err := r.hive.Get(ctx, client.ObjectKey{Namespace: clustersyncs.Items[0].Namespace, Name: clustersyncs.Items[0].Name}, clustersync)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		for _, condition := range clustersync.Status.Conditions {
			if condition.Type == hiveinternalv1alpha1.ClusterSyncFailed {
				log.Printf("clustersync: %s/%s has status: %s", clustersync.Namespace, clustersync.Name, condition.Reason)
				if condition.Status == corev1.ConditionFalse {
					return true, nil
				}
				log.Printf("clustersync: %s/%s has status: %s and message: %s, you may want to take a closer look", clustersync.Namespace, clustersync.Name, condition.Reason, condition.Message)
				return false, nil
			}
		}

		log.Printf("clustersync: %s/%s has no status condition yet, continuing to wait", clustersync.Namespace, clustersync.Name)
		return false, nil
	}); err != nil {
		return err
	}

	return nil
}
