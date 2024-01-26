package cluster

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const Namespace = "openshift-monitoring"

var detachStuckVolumeInput struct {
	Region   []string
	VolumeId []string
}

type detachStuckVolumeOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
}

func newCmdDetachStuckVolume() *cobra.Command {
	ops := newdetachStuckVolumeOptions()
	detachstuckvolumeCmd := &cobra.Command{
		Use:               "detach-stuck-volume",
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

	err = volIdRegion(clientset, Namespace, "")
	if err != nil {
		return err
	}

	// If the volIdRegion found no pv is utilized by non running state pod. Which make global variable nil.
	// Thus there's a panic exit. In order to prevent it we're using following logic to prevent panic exit.
	if len(detachStuckVolumeInput.Region) == 0 && len(detachStuckVolumeInput.VolumeId) == 0 {
		return fmt.Errorf("there's no pv utilized by non running state pod in cluster: %s\nNo action prequired", d.clusterID)
	}

	if len(detachStuckVolumeInput.Region) != 1 {
		return fmt.Errorf("Got more than one region value: %v", len(detachStuckVolumeInput.Region))
	}
	//fmt.Println(detachStuckVolumeInput.Region[0])

	//fmt.Println(detachStuckVolumeInput.VolumeId)

	// aws ec2 detach-volume --volume-id $VOLUME_ID --region $REGION --force
	// WiP - Need to convert above cmd to function once volIdRegion gets completed

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	cfg, err := osdCloud.CreateAWSV2Config(ocmClient, d.cluster)
	if err != nil {
		return err
	}
	awsClient := ec2.NewFromConfig(cfg)

	for _, Volid := range detachStuckVolumeInput.VolumeId {
		_, err := awsClient.DetachVolume(context.TODO(), &ec2.DetachVolumeInput{VolumeId: &Volid})

		if err != nil {
			return fmt.Errorf("failed to detach %s: %s\n", *&Volid, err)
		}
	}

	return nil

}

// Following function gets the volumeID & region of pv for non running state pod & value into global variable
func volIdRegion(clientset *kubernetes.Clientset, namespace, selector string) error {

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace '%s'", Namespace)

	}
	pV, err := clientset.CoreV1().PersistentVolumes().List(context.TODO(), v1.ListOptions{})

	if err != nil {
		return fmt.Errorf("failed to list pv in namespace '%s'", Namespace)

	}

	for _, pod := range pods.Items {
		if pod.Status.Phase != "Running" { // IMP: We only need to pass the following condition for non running state pods. Will un-comment once the testing is complete.
			for _, volume := range pod.Spec.Volumes {
				if volume.PersistentVolumeClaim != nil {
					for _, pVol := range pV.Items {
						if pVol.Spec.AWSElasticBlockStore != nil {
							// Most of the cluster return AWSElasticBlockStore as nil. Could write code, not sure what'll be actual response.
							fmt.Println("Gathering info from AWSElastic")
							// Logic -

						} else if pVol.Spec.CSI != nil {
							for _, pvItems := range pV.Items {
								for _, volumeNodeAffinity := range pvItems.Spec.NodeAffinity.Required.NodeSelectorTerms {
									for _, reg := range volumeNodeAffinity.MatchExpressions {
										detachStuckVolumeInput.Region = removeDuplicates(reg.Values)
										vId := append(detachStuckVolumeInput.VolumeId, pvItems.Spec.CSI.VolumeHandle)
										detachStuckVolumeInput.VolumeId = removeDuplicates(vId)

									}
								}

							}
						}

					}
				}
			}
		}

	}

	return nil
}

// Since we're using for loop in above code. It's adding duplicate entry to global variable.
// To prevent it we're using following function
func removeDuplicates(s []string) []string {
	bucket := make(map[string]bool)
	var result []string
	for _, str := range s {
		if _, ok := bucket[str]; !ok {
			bucket[str] = true
			result = append(result, str)
		}
	}
	return result
}
