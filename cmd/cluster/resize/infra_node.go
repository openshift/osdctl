package resize

import (
	"context"
	"fmt"
	"log"
	"time"

	hivev1 "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	twentyMinuteTimeout   = 20 * time.Minute
	twentySecondIncrement = 20 * time.Second
)

func newCmdResizeInfra() *cobra.Command {
	r := &Resize{}

	infraResizeCmd := &cobra.Command{
		Use:   "infra",
		Short: "Resize an OSD/ROSA cluster's infra nodes",
		Long: `Resize an OSD/ROSA cluster's infra nodes

  This command automates most of the "machinepool dance" to safely resize infra nodes for production classic OSD/ROSA 
  clusters. This DOES NOT work in non-production due to environmental differences.

  Remember to follow the SOP for preparation and follow up steps:

    https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md
`,
		Example: `
  # Automatically vertically scale infra nodes to the next size
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID}

  # Resize infra nodes to a specific instance type
  osdctl cluster resize infra --cluster-id ${CLUSTER_ID} --instance-type "r5.xlarge"
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return r.RunInfra(context.Background())
		},
	}

	infraResizeCmd.Flags().StringVarP(&r.clusterId, "cluster-id", "C", "", "OCM internal cluster id to resize infra nodes for.")
	infraResizeCmd.Flags().StringVar(&r.instanceType, "instance-type", "", "(optional) AWS EC2 instance type to resize the infra nodes to.")

	infraResizeCmd.MarkFlagRequired("cluster-id")

	return infraResizeCmd
}

func (r *Resize) RunInfra(ctx context.Context) error {
	if err := r.New(r.clusterId); err != nil {
		return err
	}

	log.Printf("resizing infra nodes for %s", r.clusterId)
	originalMp, err := r.getInfraMachinePool(ctx)
	if err != nil {
		return err
	}

	newMp, err := r.embiggenMachinePool(originalMp)
	if err != nil {
		return err
	}
	tempMp := newMp.DeepCopy()
	tempMp.Name = fmt.Sprintf("%s2", tempMp.Name)
	tempMp.Spec.Name = fmt.Sprintf("%s2", tempMp.Spec.Name)

	// Create the temporary machinepool
	log.Printf("planning to resize to machine type %s", tempMp.Spec.Platform.AWS.InstanceType)
	log.Printf("[REMINDER] follow the preparation tasks described in https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md#prepare")
	if !utils.ConfirmPrompt() {
		log.Printf("exiting")
		return nil
	}

	log.Printf("creating temporary machinepool %s, with machine type %s", tempMp.Name, tempMp.Spec.Platform.AWS.InstanceType)
	if err := r.hiveAdmin.Create(ctx, tempMp); err != nil {
		return err
	}

	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}
		selector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			return false, err
		}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			return false, err
		}

		readyNodes := 0
		log.Printf("waiting for %d infra nodes to be reporting Ready", int(*originalMp.Spec.Replicas)*2)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady {
					if cond.Status == corev1.ConditionTrue {
						readyNodes++
						log.Printf("found node %s reporting Ready", node.Name)
					}
				}
			}
		}

		switch {
		case readyNodes >= int(*originalMp.Spec.Replicas)*2:
			return true, nil
		default:
			log.Printf("found %d infra nodes reporting Ready, continuing to wait", readyNodes)
			return false, nil
		}
	}); err != nil {
		return err
	}

	// Delete original machinepool
	log.Printf("deleting original machinepool %s, with machine type %s", originalMp.Name, originalMp.Spec.Platform.AWS.InstanceType)
	if err := r.hiveAdmin.Delete(ctx, originalMp); err != nil {
		return err
	}

	// Wait for original machines to delete
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		mp := &hivev1.MachinePool{}
		err := r.hive.Get(ctx, client.ObjectKey{Namespace: originalMp.Namespace, Name: originalMp.Name}, mp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		log.Printf("original machinepool %s/%s still exists, continuing to wait", originalMp.Namespace, originalMp.Name)
		return false, nil
	}); err != nil {
		return err
	}

	// Create new permanent machinepool
	log.Printf("creating new machinepool %s, with machine type %s", newMp.Name, newMp.Spec.Platform.AWS.InstanceType)
	if err := r.hiveAdmin.Create(ctx, newMp); err != nil {
		return err
	}

	// Wait for new permanent machines to become nodes
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}
		selector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			return false, err
		}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			return false, err
		}

		readyNodes := 0
		log.Printf("waiting for %d infra nodes to be reporting Ready", int(*originalMp.Spec.Replicas)*2)
		for _, node := range nodes.Items {
			for _, cond := range node.Status.Conditions {
				if cond.Type == corev1.NodeReady {
					if cond.Status == corev1.ConditionTrue {
						readyNodes++
						log.Printf("found node %s reporting Ready", node.Name)
					}
				}
			}
		}

		switch {
		case readyNodes >= int(*originalMp.Spec.Replicas)*2:
			return true, nil
		default:
			log.Printf("found %d infra nodes reporting Ready, continuing to wait", readyNodes)
			return false, nil
		}
	}); err != nil {
		return err
	}

	// Delete temp machinepool
	log.Printf("deleting temporary machinepool %s, with machine type %s", tempMp.Name, tempMp.Spec.Platform.AWS.InstanceType)
	if err := r.hiveAdmin.Delete(ctx, tempMp); err != nil {
		return err
	}

	// Wait for temporary machinepool to delete
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		mp := &hivev1.MachinePool{}
		err := r.hive.Get(ctx, client.ObjectKey{Namespace: tempMp.Namespace, Name: tempMp.Name}, mp)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}

		log.Printf("temporary machinepool %s/%s still exists, continuing to wait", tempMp.Namespace, tempMp.Name)
		return false, nil
	}); err != nil {
		return err
	}

	// Wait for infra node count to return to normal
	log.Printf("waiting for infra node count to return to: %d", int(*originalMp.Spec.Replicas))
	if err := wait.PollImmediate(twentySecondIncrement, twentyMinuteTimeout, func() (bool, error) {
		nodes := &corev1.NodeList{}
		selector, err := labels.Parse("node-role.kubernetes.io/infra=")
		if err != nil {
			return false, err
		}

		if err := r.client.List(ctx, nodes, &client.ListOptions{LabelSelector: selector}); err != nil {
			return false, err
		}

		switch len(nodes.Items) {
		case int(*originalMp.Spec.Replicas):
			log.Printf("found %d infra nodes, infra resize complete", len(nodes.Items))
			return true, nil
		default:
			log.Printf("found %d infra nodes, continuing to wait", len(nodes.Items))
			return false, nil
		}
	}); err != nil {
		return err
	}

	log.Printf("[REMINDER] follow the cleanup tasks in https://github.com/openshift/ops-sop/blob/master/v4/howto/resize-infras-workers.md#sending-the-all-clear")
	return nil
}

func (r *Resize) getInfraMachinePool(ctx context.Context) (*hivev1.MachinePool, error) {
	ns := &corev1.NamespaceList{}
	selector, err := labels.Parse(fmt.Sprintf("api.openshift.com/id=%s", r.clusterId))
	if err != nil {
		return nil, err
	}

	if err := r.hive.List(ctx, ns, &client.ListOptions{LabelSelector: selector, Limit: 1}); err != nil {
		return nil, err
	}
	if len(ns.Items) != 1 {
		return nil, fmt.Errorf("expected 1 namespace, found %d namespaces with tag: api.openshift.com/id=%s", len(ns.Items), r.clusterId)
	}

	log.Printf("found namespace: %s", ns.Items[0].Name)

	mpList := &hivev1.MachinePoolList{}
	if err := r.hive.List(ctx, mpList, &client.ListOptions{Namespace: ns.Items[0].Name}); err != nil {
		return nil, err
	}

	for _, mp := range mpList.Items {
		mp := mp
		if mp.Spec.Name == "infra" {
			log.Printf("found machinepool %s, with machine type %s", mp.Name, mp.Spec.Platform.AWS.InstanceType)
			return &mp, nil
		}
	}

	return nil, fmt.Errorf("did not find the infra machinepool in namespace: %s", ns.Items[0].Name)
}

func (r *Resize) embiggenMachinePool(mp *hivev1.MachinePool) (*hivev1.MachinePool, error) {
	embiggen := map[string]string{
		"m5.xlarge":  "r5.xlarge",
		"m5.2xlarge": "r5.2xlarge",
		"r5.xlarge":  "r5.2xlarge",
		"r5.2xlarge": "r5.4xlarge",
		"r5.4xlarge": "r5.8xlarge",
	}

	newMp := &hivev1.MachinePool{}
	mp.DeepCopyInto(newMp)

	// Unset fields we want to be regenerated
	newMp.CreationTimestamp = metav1.Time{}
	newMp.Finalizers = []string{}
	newMp.ResourceVersion = ""
	newMp.Generation = 0
	newMp.SelfLink = ""
	newMp.UID = ""
	newMp.Status = hivev1.MachinePoolStatus{}

	// Update instance type sizing
	if r.instanceType != "" {
		log.Printf("using override instance type: %s", r.instanceType)
		newMp.Spec.Platform.AWS.InstanceType = r.instanceType
	} else {
		if _, ok := embiggen[mp.Spec.Platform.AWS.InstanceType]; !ok {
			return nil, fmt.Errorf("resizing instance type %s not supported", mp.Spec.Platform.AWS.InstanceType)
		}
		newMp.Spec.Platform.AWS.InstanceType = embiggen[mp.Spec.Platform.AWS.InstanceType]
	}

	return newMp, nil
}
