package access

import (
	"context"
	"fmt"
	"os"
	fpath "path/filepath"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	osdctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func newCmdCleanup(client *k8s.LazyClient, streams genericclioptions.IOStreams) *cobra.Command {
	ops := newCleanupAccessOptions(client, streams)
	cleanupCmd := &cobra.Command{
		Use:   "cleanup --cluster-id <cluster-identifier>",
		Short: "Drop emergency access to a cluster",
		Long:  "Relinquish emergency access from the given cluster. If the cluster is PrivateLink or Private\nService Connect (PSC), it deletes all jump pods in the cluster's namespace (because of this, you\nmust be logged into the hive shard when dropping access for PrivateLink/PSC clusters). For other\nclusters, the $KUBECONFIG environment variable is unset, if applicable.",
		Example: `  # Drop emergency access to a cluster
  osdctl cluster break-glass cleanup --cluster-id ${CLUSTER_ID}`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(cleanupCmdComplete(cmd))
			cmdutil.CheckErr(ops.Run(cmd))
		},
	}
	cleanupCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "[Mandatory] Provide the Internal ID of the cluster")
	cleanupCmd.Flags().StringVar(&ops.reason, "reason", "", "[Mandatory for PrivateLink/PSC clusters] The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	cleanupCmd.Flags().StringVar(&ops.hiveOcmUrl, "hive-ocm-url", "", "(optional) OCM environment URL for Hive operations. Aliases: 'production', 'staging', 'integration'. This only changes how the Hive cluster is resolved; the target cluster still comes from the current/default OCM environment.")

	_ = cleanupCmd.MarkFlagRequired("cluster-id")

	return cleanupCmd
}

func cleanupCmdComplete(cmd *cobra.Command) error {
	clusterID, _ := cmd.Flags().GetString("cluster-id")
	if clusterID == "" {
		return cmdutil.UsageErrorf(cmd, "The cluster-id flag is required")
	}
	if err := osdctlutil.IsValidClusterKey(clusterID); err != nil {
		return err
	}

	hiveOcmUrl, _ := cmd.Flags().GetString("hive-ocm-url")
	if hiveOcmUrl != "" {
		if _, err := osdctlutil.ValidateAndResolveOcmUrl(hiveOcmUrl); err != nil {
			return fmt.Errorf("invalid --hive-ocm-url: %w", err)
		}
	}

	return nil
}

// cleanupAccessOptions contains the objects and information required to drop access to a cluster
type cleanupAccessOptions struct {
	reason     string
	clusterID  string
	hiveOcmUrl string

	genericclioptions.IOStreams
	kubeCli *k8s.LazyClient
}

// newCleanupAccessOptions creates a cleanupAccessOptions object
func newCleanupAccessOptions(client *k8s.LazyClient, streams genericclioptions.IOStreams) cleanupAccessOptions {
	c := cleanupAccessOptions{
		IOStreams: streams,
		kubeCli:   client,
	}
	return c
}

// Println appends a newline then prints the given msg using the cleanupAccessOptions' IOStreams
func (c *cleanupAccessOptions) Println(msg string) {
	osdctlutil.StreamPrintln(c.IOStreams, msg)
}

// Println prints the given msg using the cleanupAccessOptions' IOStreams
func (c *cleanupAccessOptions) Print(msg string) {
	osdctlutil.StreamPrint(c.IOStreams, msg)
}

// Println appends a newline then prints the given error msg using the cleanupAccessOptions' IOStreams
func (c *cleanupAccessOptions) Errorln(msg string) {
	osdctlutil.StreamErrorln(c.IOStreams, msg)
}

// Readln reads a single line of user input using the cleanupAccessOptions' IOStreams. User input is returned with all
// proceeding and following whitespace trimmed
func (c *cleanupAccessOptions) Readln() (string, error) {
	in, err := osdctlutil.StreamRead(c.IOStreams, '\n')
	return strings.TrimSpace(in), err
}

// Run executes the 'cleanup' access subcommand
func (c *cleanupAccessOptions) Run(cmd *cobra.Command) error {
	conn, err := osdctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		cmdutil.CheckErr(conn.Close())
	}()

	cluster, err := osdctlutil.GetCluster(conn, c.clusterID)
	if err != nil {
		return err
	}
	c.Println(fmt.Sprintf("Dropping access to cluster '%s'", cluster.Name()))
	isPscCluster := cluster.GCP().PrivateServiceConnect().ServiceAttachmentSubnet() != ""
	if cluster.AWS().PrivateLink() || isPscCluster {
		return c.dropPrivateLinkAccess(cluster, conn)
	} else {
		return c.dropLocalAccess(cluster)
	}
}

// dropPrivateLinkAccess removes access to a PrivateLink cluster.
// This primarily consists of deleting any jump pods found to be running against the cluster in hive.
func (c *cleanupAccessOptions) dropPrivateLinkAccess(cluster *clustersmgmtv1.Cluster, conn *sdk.Connection) error {
	if c.reason == "" {
		c.Errorln("flag \"reason\" not set and is required when Cluster is PrivateLink or Private Service Connect")
		return fmt.Errorf("flag \"reason\" not set and is required when Cluster is PrivateLink or Private Service Connect")
	}

	var hiveClient kclient.Client
	if c.hiveOcmUrl != "" {
		hiveOCM, err := osdctlutil.CreateConnectionWithUrl(c.hiveOcmUrl)
		if err != nil {
			return fmt.Errorf("failed to create hive OCM connection with URL '%s': %w", c.hiveOcmUrl, err)
		}
		defer hiveOCM.Close()

		hive, err := osdctlutil.GetHiveClusterWithConn(cluster.ID(), conn, hiveOCM)
		if err != nil {
			return fmt.Errorf("failed to retrieve hive shard for %q (OCM URL:'%s'): %w", cluster.ID(), c.hiveOcmUrl, err)
		}

		hiveClient, err = k8s.NewAsBackplaneClusterAdminWithConn(hive.ID(), kclient.Options{Scheme: scheme.Scheme}, hiveOCM, c.reason, "Elevation required to clean break-glass on PrivateLink/PSC Clusters")
		if err != nil {
			return fmt.Errorf("failed to login to hive shard %q (OCM URL:'%s'): %w", hive.Name(), c.hiveOcmUrl, err)
		}
	} else {
		c.kubeCli.Impersonate("backplane-cluster-admin", c.reason, "Elevation required to clean break-glass on PrivateLink/PSC Clusters")
		hiveClient = c.kubeCli
	}

	c.Println("Cluster is PrivateLink or Private Service Connect - removing jump pods in the cluster's namespace.")
	ns, err := getClusterNamespace(hiveClient, cluster.ID())
	if err != nil {
		c.Errorln("Failed to retrieve cluster namespace")
		if c.hiveOcmUrl == "" {
			c.Errorln("Hint: if the cluster's hive shard is in a different OCM environment, use --hive-ocm-url (e.g. --hive-ocm-url production)")
		}
		return err
	}

	// Generate label selector to only target pods w/ matching jump pod label
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{jumpPodLabelKey: cluster.ID()}}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		c.Errorln("Failed to convert labelSelector to selector")
		return err
	}

	listOpts := kclient.ListOptions{Namespace: ns.Name, LabelSelector: selector}
	pods := corev1.PodList{}
	err = hiveClient.List(context.TODO(), &pods, &listOpts)
	if err != nil {
		c.Errorln(fmt.Sprintf("Failed to list pods in cluster namespace '%s'", ns.Name))
		return err
	}

	numPods := len(pods.Items)
	if numPods == 0 {
		c.Println(fmt.Sprintf("No jump pods found running in namespace '%s'.", ns.Name))
		c.Println("Access has been dropped.")
		return nil
	}

	c.Println("")
	c.Println(fmt.Sprintf("This will delete %d pods in the namespace '%s'", numPods, ns.Name))
	for _, pod := range pods.Items {
		c.Println(fmt.Sprintf("- %s", pod.Name))
	}
	c.Println("")
	c.Print("Continue? [y/N] ")
	input, err := c.Readln()
	if err != nil {
		c.Errorln("Failed to read user input")
		return err
	}
	if isAffirmative(input) {
		pod := corev1.Pod{}
		err = hiveClient.DeleteAllOf(context.TODO(), &pod, &kclient.DeleteAllOfOptions{ListOptions: listOpts})
		if err != nil {
			c.Errorln("Failed to delete pod(s)")
			return err
		}

		c.Println(fmt.Sprintf("Waiting for %d pod(s) to terminate", numPods))
		err = wait.PollImmediate(jumpPodPollInterval, jumpPodPollTimeout, func() (done bool, err error) {
			// For some reason, we have to recreate the podList after deleting the pods, otherwise the listOpts don't filter properly,
			// and we end up waiting for irrelevant pods. I've tried reproducing this bug in other places, but I haven't been able to
			// figure it out. If someone does, please fix it.
			pods := corev1.PodList{}
			err = hiveClient.List(context.TODO(), &pods, &listOpts)
			if err != nil || len(pods.Items) != 0 {
				return false, err
			}
			return true, nil
		})
		if err != nil {
			c.Errorln("Error while waiting for pods to terminate")
			return err
		}
		c.Println("Access has been dropped.")
	} else {
		c.Println("Access has not been dropped.")
	}
	return nil
}

// dropLocalAccess removes access to a non-PrivateLink cluster.
// Basically it just unsets KUBECONFIG if it appears to be set to the given cluster, since we can't make assumptions
// around local files.
func (c *cleanupAccessOptions) dropLocalAccess(cluster *clustersmgmtv1.Cluster) error {
	c.Println("Unsetting $KUBECONFIG for cluster")
	kubeconfigPath, found := os.LookupEnv("KUBECONFIG")
	if !found {
		c.Errorln("'KUBECONFIG' unset. Access appears to have already been dropped.")
		return nil
	}

	kubeconfigFileName := fpath.Base(kubeconfigPath)
	if !strings.Contains(kubeconfigFileName, cluster.Name()) {
		c.Errorln(fmt.Sprintf("'KUBECONFIG' set to '%s', which does not seem to be the kubeconfig for '%s'. Access assumed to have already been dropped.", kubeconfigFileName, cluster.Name()))
		c.Errorln("(If you think this is a mistake, you can still manually drop access by running `unset KUBECONFIG` in the affected terminals)")
		return nil
	}

	c.Print(fmt.Sprintf("$KUBECONFIG set to '%s'. Unset it? [y/N]", kubeconfigPath))
	input, err := c.Readln()
	if err != nil {
		c.Errorln("Failed to read user input")
		return err
	}

	if isAffirmative(input) {
		c.Println("Unsetting $KUBECONFIG")
		err = os.Unsetenv("KUBECONFIG")
		if err != nil {
			c.Errorln("Failed to unset $KUBECONFIG")
			return err
		}
		c.Println("Successfully unset $KUBECONFIG.")
	}

	c.Println("Access has been dropped.")
	return nil
}
