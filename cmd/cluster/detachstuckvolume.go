package cluster

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const Namespace = "openshift-monitoring"

var detachStuckVolumeInput struct {
	VolumeId []string
}

type detachStuckVolumeOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
	reason    string
}

func newCmdDetachStuckVolume() *cobra.Command {
	ops := &detachStuckVolumeOptions{}
	detachstuckvolumeCmd := &cobra.Command{
		Use:               "detach-stuck-volume --cluster-id <cluster-identifier>",
		Short:             "Detach openshift-monitoring namespace's volume from a cluster forcefully",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.detachVolume())
		},
	}

	detachstuckvolumeCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Provide internal ID of the cluster")
	detachstuckvolumeCmd.Flags().StringVar(&ops.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usually an OHSS or PD ticket)")
	_ = detachstuckvolumeCmd.MarkFlagRequired("cluster-id")
	_ = detachstuckvolumeCmd.MarkFlagRequired("reason")

	return detachstuckvolumeCmd
}

func (o *detachStuckVolumeOptions) detachVolume() error {
	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()
	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}
	o.cluster = cluster
	o.clusterID = cluster.ID()
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("this command is only available for AWS clusters")
	}

	elevationReasons := []string{
		o.reason,
		"Detach stuck volume in openshift-monitoring",
	}
	_, _, clientset, err := common.GetKubeConfigAndClient(o.clusterID, elevationReasons...)

	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", o.clusterID, err)
	}

	err = getVolumeID(clientset, Namespace)
	if err != nil {
		return err
	}

	// If the volIdRegion found no pv is utilized by non running state pod. Which make global variable nil.
	// Thus there's a panic exit. In order to prevent it we're using following logic to prevent panic exit.
	if len(detachStuckVolumeInput.VolumeId) == 0 {
		return fmt.Errorf("there's no pv utilized by non running state pod in cluster: %s\nNo action required", o.clusterID)
	}

	log.Printf("The volume id are %v\n", detachStuckVolumeInput.VolumeId)

	// Aws fuction to detach volume of no running state pod's using it's volume id
	cfg, err := osdCloud.CreateAWSV2Config(connection, o.cluster)
	if err != nil {
		return err
	}
	awsClient := ec2.NewFromConfig(cfg)

	for _, Volid := range detachStuckVolumeInput.VolumeId {
		_, err := awsClient.DetachVolume(context.TODO(), &ec2.DetachVolumeInput{VolumeId: &Volid})

		if err != nil {
			return fmt.Errorf("failed to detach %s: %s", *&Volid, err)
		}
		log.Printf("%s has been detached", Volid)
	}

	return nil
}

// Following function gets the volumeID & region of pv for non running state pod & value into global variable
func getVolumeID(clientset *kubernetes.Clientset, namespace string) error {

	var pvClaim []string
	var pVolume []string

	// Getting pod objects for non-running state pod
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{FieldSelector: "status.phase!=Running"})

	if err != nil {
		return fmt.Errorf("failed to list pods in namespace '%s'", Namespace)

	}

	// Getting pvc name of non-running state pod and passing it into pvClaim slice
	for _, pod := range pods.Items {

		for _, pvC := range pod.Spec.Volumes {
			if pvC.PersistentVolumeClaim != nil {
				pvClaim = append(pvClaim, pvC.PersistentVolumeClaim.ClaimName)
			}
		}
	}

	// Gathering persistent volume obj from above gathered pvc & passing it to pVolume slice
	for _, singlePvc := range pvClaim {
		pvC, err := clientset.CoreV1().PersistentVolumeClaims(namespace).List(context.TODO(), v1.ListOptions{FieldSelector: "metadata.name=" + singlePvc})
		if err != nil {
			return fmt.Errorf("failed to list pvc in namespace '%s'", Namespace)
		}
		for _, pvcObject := range pvC.Items {
			pVolume = append(pVolume, pvcObject.Spec.VolumeName)
		}
	}

	// Gathering volumeid & region from PV obj from non running state pod alone
	for _, singlePv := range pVolume {
		pV, err := clientset.CoreV1().PersistentVolumes().List(context.TODO(), v1.ListOptions{FieldSelector: "metadata.name=" + singlePv})

		if err != nil {
			return fmt.Errorf("failed to list pv in namespace '%s'", Namespace)
		}

		for _, pVol := range pV.Items {
			if pVol.Spec.AWSElasticBlockStore != nil {
				// Most of the cluster return AWSElasticBlockStore as nil.
				// Couldn't write tge logic sure what'll be actual response.
				// Also it's been deprecated in most of the clusters.
				fmt.Println("Gathering info from AWSElastic")
				// If required logic can be added below in future

			} else if pVol.Spec.CSI != nil {

				volIdCheckBool := slices.Contains(detachStuckVolumeInput.VolumeId, pVol.Spec.CSI.VolumeHandle)
				if !volIdCheckBool {
					detachStuckVolumeInput.VolumeId = append(detachStuckVolumeInput.VolumeId, pVol.Spec.CSI.VolumeHandle)
				}

			}
		}
	}
	return nil
}
