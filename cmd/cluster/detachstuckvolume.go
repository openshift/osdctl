package cluster

import (
	"context"
	"fmt"
	"strings"

	slices "golang.org/x/exp/slices"

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
	// commenting all the function for region. REASON: It seems region isn't a mandatory field in aws sdk detachStuckVolume function
	// Region []string
	VolumeId []string
}

type detachStuckVolumeOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
}

func newCmdDetachStuckVolume() *cobra.Command {
	ops := &detachStuckVolumeOptions{}
	detachstuckvolumeCmd := &cobra.Command{
		Use:               "detach-stuck-volume",
		Short:             "Detach openshift-monitoring namespace's volume from a cluster forcefully",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.detachVolume(args[0]))
		},
	}
	return detachstuckvolumeCmd

}

func (o *detachStuckVolumeOptions) detachVolume(clusterID string) error {

	err := utils.IsValidClusterKey(clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()
	cluster, err := utils.GetCluster(connection, clusterID)
	if err != nil {
		return err
	}
	o.cluster = cluster
	o.clusterID = cluster.ID()
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("this command is only available for AWS clusters")
	}

	_, _, clientset, err := common.GetKubeConfigAndClient(o.clusterID, "", "")

	if err != nil {
		return fmt.Errorf("failed to retrieve Kubernetes configuration and client for cluster with ID %s: %w", o.clusterID, err)
	}

	err = getVolumeID(clientset, Namespace, "")
	if err != nil {
		return err
	}

	// If the volIdRegion found no pv is utilized by non running state pod. Which make global variable nil.
	// Thus there's a panic exit. In order to prevent it we're using following logic to prevent panic exit.
	if len(detachStuckVolumeInput.VolumeId) == 0 {
		return fmt.Errorf("there's no pv utilized by non running state pod in cluster: %s\nNo action required", o.clusterID)
	}

	/*
		if len(detachStuckVolumeInput.Region) != 1 {
			return fmt.Errorf("got more than one region value: %v", len(detachStuckVolumeInput.Region))
		}
	*/

	//fmt.Println(detachStuckVolumeInput.Region[0])

	fmt.Printf("The volume id are %v\n", detachStuckVolumeInput.VolumeId)

	// aws ec2 detach-volume --volume-id $VOLUME_ID --region $REGION --force
	// WiP - Need to convert above cmd to function once volIdRegion gets completed
	// Tested till line 107 - Couldn't test below aws function getting priv issue. gig acc doesn't have nessary priv

	cfg, err := osdCloud.CreateAWSV2Config(connection, o.cluster)
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
func getVolumeID(clientset *kubernetes.Clientset, namespace, selector string) error {

	var pvClaim []string
	var pVolume []string

	// Getting pod objects for non-running state pod
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), v1.ListOptions{FieldSelector: "status.phase!=Running"}) // For testing function, we can change `!=` to `=`` to remove pv of running state pod

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
				//for _, volumeNodeAffinity := range pVol.Spec.NodeAffinity.Required.NodeSelectorTerms {
				// _, reg := range volumeNodeAffinity.MatchExpressions {

				volIdCheckBool := slices.Contains(detachStuckVolumeInput.VolumeId, pVol.Spec.CSI.VolumeHandle)
				if !volIdCheckBool {
					detachStuckVolumeInput.VolumeId = append(detachStuckVolumeInput.VolumeId, pVol.Spec.CSI.VolumeHandle)
				}

				//}
				//}
			}
		}
	}
	return nil
}
