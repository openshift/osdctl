package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type etcdOptions struct {
	nodeId string
}

// Secrets List
var secrets = []string{
	"etcd-peer-",
	"etcd-serving-",
	"etcd-serving-metrics-",
}

// Patch Constants
const (
	EtcdQuorumTurnOffPatch = `{"spec": {"unsupportedConfigOverrides": {"useUnsupportedUnsafeNonHANonProductionUnstableEtcd": true}}}`
	EtcdQuorumTurnOnPatch  = `{"spec": {"unsupportedConfigOverrides": null}}`
	EtcdForceRedeployPatch = `{"spec": {"forceRedeploymentReason": "single-master-recovery-%s"}}`
)

func newCmdEtcdMemberReplacement() *cobra.Command {
	opts := &etcdOptions{}
	replaceCmd := &cobra.Command{
		Use:               "etcd-member-replace <cluster-id>",
		Short:             "Replaces an unhealthy etcd node",
		Long:              `Replaces an unhealthy ectd node using the member id provided`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(opts.EtcdReplaceMember(args[0]))
		},
	}
	replaceCmd.Flags().StringVar(&opts.nodeId, "node", "", "Node ID (required)")
	replaceCmd.MarkFlagRequired("node")
	return replaceCmd
}

func (opts *etcdOptions) EtcdReplaceMember(clusterId string) error {
	kubeCli, kconfig, clientset, err := getKubeConfigAndClient(clusterId)
	if err != nil {
		return err
	}

	listOptions := metav1.ListOptions{
		LabelSelector: EtcdLabelSelector,
	}

	if opts.nodeId == "" {
		return fmt.Errorf("node name cannot be blank. Please provide node using --node flag")
	}

	fmt.Println("[INFO] This operation will run with the \"backplane-cluster-admin\" role")
	// Get the etcd pods
	pods, err := clientset.CoreV1().Pods(EtcdNamespaceName).List(context.TODO(), listOptions)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Status.ContainerStatuses {
			if c.State.Waiting != nil && (c.State.Waiting.Reason == "CrashLoopBackOff" || c.State.Waiting.Reason == "Error") {
				podName := "etcd-" + opts.nodeId
				if podName != pod.ObjectMeta.Name {
					return fmt.Errorf("the etcd member seems to be healthy or is not present. Please run health-check again")
				}
				err := opts.ReplaceEtcdMember(kubeCli, kconfig, clientset, pod.ObjectMeta.Name)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	return fmt.Errorf("none of the etcd members seems to be unhealthy. Please verify once again")
}

func (opts *etcdOptions) ReplaceEtcdMember(kubeCli client.Client, kconfig *rest.Config, clientset *kubernetes.Clientset, pod string) error {
	// Remove the etcd member
	fmt.Printf("[INFO] Starting etcd member replacement for pod %s", pod)
	err := opts.removeEtcdMember(kconfig, clientset, pod)
	if err != nil {
		return err
	}

	// Turn off the quorum guard
	fmt.Println("[INFO] Turning the quorum guard off")
	err = patchEtcd(kubeCli, EtcdQuorumTurnOffPatch)
	if err != nil {
		fmt.Println("[ERROR] Could not Turn the quorum guard off. Refer error below")
		return err
	}

	// Delete the secrets for the unhealthy etcd member
	fmt.Println("[INFO] Deleting the secrets for the unhealthy etcd member")
	err = opts.removeEtcdSecrets(clientset)
	if err != nil {
		fmt.Println("[ERROR] Could not delete the secrets for the unhealthy etcd member. Refer error below")
		return err
	}

	// Force etcd redeployment.
	fmt.Println("[INFO] Forcing etcd redeployment")
	timeStamp := time.Now().Format(time.RFC3339Nano)
	patch := fmt.Sprintf(EtcdForceRedeployPatch, timeStamp)
	err = patchEtcd(kubeCli, patch)
	if err != nil {
		fmt.Println("[ERROR] Could not force etcd redeployment. Refer error below")
		return err
	}

	// Turn the quorum guard back
	fmt.Println("[INFO] Turning the quorum guard back on")
	err = patchEtcd(kubeCli, EtcdQuorumTurnOnPatch)
	if err != nil {
		fmt.Println("[ERROR]  Could not Turn the quorum guard back on. Refer error below")
		return err
	}

	fmt.Println("The etcd member has been successfully replaced. Please verify by running health check after few minutes.")
	return nil
}

func (opts *etcdOptions) removeEtcdMember(kconfig *rest.Config, clientset *kubernetes.Clientset, pod string) error {
	fmt.Println()
	cmd := "etcdctl member list -w table | grep " + opts.nodeId + " | awk '{ print $2 }'"
	memberId, err := Etcdctlhealth(kconfig, clientset, cmd, pod)
	if err != nil {
		return err
	}
	memberId = strings.TrimSpace(memberId)
	fmt.Printf("[INFO] Replacing pod %s having member id %s.\n", pod, memberId)

	prompt := utils.ConfirmPrompt()
	if !prompt {
		return fmt.Errorf("operation cancelled by user")
	}

	remove_cmd := "etcdctl member remove " + memberId
	output, err := Etcdctlhealth(kconfig, clientset, remove_cmd, pod)
	if err != nil {
		fmt.Println("[ERROR] Could not replace pod. Refer error below")
		return err
	}
	fmt.Println(output)
	return nil
}

func patchEtcd(kubeCli client.Client, patch string) error {
	etcdCR := &operatorv1.Etcd{}
	err := kubeCli.Get(context.TODO(), client.ObjectKey{Name: "cluster"}, etcdCR)
	if err != nil {
		return err
	}
	err = kubeCli.Patch(context.TODO(), etcdCR, client.RawPatch(types.MergePatchType, []byte(patch)), &client.PatchOptions{})
	if err != nil {
		return err
	}
	return nil
}

func (opts *etcdOptions) removeEtcdSecrets(clientset *kubernetes.Clientset) error {
	for _, secret := range secrets {
		name := secret + opts.nodeId
		err := clientset.CoreV1().Secrets("openshift-etcd").Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
