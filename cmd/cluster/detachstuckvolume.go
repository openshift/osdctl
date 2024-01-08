package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const NameSpace = "openshift-monitoring"

type detachStuckVolumeOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
}

// detachstuckvolumeCmd represents the detachstuckvolume command
func newCmdDetachStuckVolume() *cobra.Command {
	ops := newdetachStuckVolumeOptions()
	detachstuckvolumeCmd := &cobra.Command{
		Use:               "detachstuckvolume",
		Short:             "Detach openshift-monitoring namespace's volume from a cluster forcefully",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	detachstuckvolumeCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "c", "", "The internal ID of the cluster to perform actions on")
	detachstuckvolumeCmd.MarkFlagRequired("cluster-id")
	return detachstuckvolumeCmd

}

func newdetachStuckVolumeOptions() *detachStuckVolumeOptions {
	return &detachStuckVolumeOptions{}
}

func (d *detachStuckVolumeOptions) complete(cmd *cobra.Command, args []string) error {

	err := utils.IsValidClusterKey(d.clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()
	cluster, err := utils.GetCluster(connection, d.clusterID)
	if err != nil {
		return err
	}
	d.cluster = cluster
	d.clusterID = cluster.ID()
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return errors.New("this command is only available for AWS clusters")
	}

	return nil
}

func (d *detachStuckVolumeOptions) run() error {

	_, _, clientset, err := getKubeConfigAndClient(d.clusterID)

	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", d.clusterID, err)
	}

	err = volIdRegion(clientset, "openshift-monitoring", "")
	if err != nil {
		return err
	}

	// aws ec2 detach-volume --volume-id $VOLUME_ID --region $REGION --force
	// WiP - Need to convert above cmd to function once volIdRegion gets completed

	return nil

}

func volIdRegion(clientset *kubernetes.Clientset, namespace, selector string) error {

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace '%s'", NameSpace)

	}

	for _, pod := range pods.Items {
		//if pod.Status.Phase != "Running" {  // IMP: We only need to pass the following condition for non running state pods. Will un-comment once the testing is complete.
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.VolumeSource.AWSElasticBlockStore != nil {
				//Didn't get valid cluster from stg enviroment to pass above condition. AWSElasticBlockStore response is nil in all case which i tested.
				fmt.Print("test")
			}
		}
	}

	return nil
}

// Thought to created an object from volIdRegion func & then pass it aws cmd.
/*
type poPvPvC struct {
	volumeID string
	Region      string
}
*/
