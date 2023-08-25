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
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type flagOptions struct {
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

func newCmdEtcdMemberReplacement(kubeCli client.Client, flags *genericclioptions.ConfigFlags) *cobra.Command {
	opts := &flagOptions{}
	replaceCmd := &cobra.Command{
		Use:               "etcd-member-replacement",
		Short:             "Replaces an unhealthy etcd node",
		Long:              `Replaces an unhealthy ectd node using the member id provided`,
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(EtcdReplaceMember(kubeCli, *opts, flags))
		},
	}
	replaceCmd.Flags().StringVar(&opts.nodeId, "node", "", "Node ID")
	return replaceCmd
}

func EtcdReplaceMember(kubeCli client.Client, opts flagOptions, flags *genericclioptions.ConfigFlags) error {
	kconfig, clientset, err := getKubeConfigAndClientSet()
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
					return fmt.Errorf("the etcd member seems to be healthy. Please run health-check again")
				}
				err := ReplaceEtcdMember(kubeCli, kconfig, clientset, flags, opts, pod.ObjectMeta.Name)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	return fmt.Errorf("none of the etcd members seems to be unhealthy. Please verify once again")
}

func ReplaceEtcdMember(kubeCli client.Client, kconfig *rest.Config, clientset *kubernetes.Clientset, flags *genericclioptions.ConfigFlags, opts flagOptions, pod string) error {
	// Remove the etcd member
	err := removeEtcdMember(kconfig, clientset, pod, opts)
	if err != nil {
		return err
	}

	// Turn off the quorum guard
	fmt.Println("[INFO] Turning the quorum guard off")
	err = patchEtcdQuorum(kubeCli, flags, EtcdQuorumTurnOffPatch)
	if err != nil {
		return err
	}

	// Delete the secrets for the unhealthy etcd member
	err = removeEtcdSecrets(clientset, opts)
	if err != nil {
		return err
	}

	// Force etcd redeployment.
	fmt.Println("[INFO] Forcing etcd redeployment")
	timeStamp := time.Now().Format(time.RFC3339Nano)
	patch := fmt.Sprintf(EtcdForceRedeployPatch, timeStamp)
	err = patchEtcdQuorum(kubeCli, flags, patch)
	if err != nil {
		return err
	}

	// Turn the quorum guard back
	fmt.Println("[INFO] Turning the quorum guard back on")
	err = patchEtcdQuorum(kubeCli, flags, EtcdQuorumTurnOnPatch)
	if err != nil {
		return err
	}

	fmt.Println("The etcd member has been successfully replaced. Please verify by running health check after few minutes.")
	return nil
}

func removeEtcdMember(kconfig *rest.Config, clientset *kubernetes.Clientset, pod string, opts flagOptions) error {
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
		fmt.Println("Could not replace pod. Refer error below")
		return err
	}
	fmt.Println(output)
	return nil
}

func patchEtcdQuorum(kubeCli client.Client, flags *genericclioptions.ConfigFlags, patch string) error {
	etcdCR := &operatorv1.Etcd{}
	flags.Impersonate = &BackplaneClusterAdmin

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

func removeEtcdSecrets(clientset *kubernetes.Clientset, opts flagOptions) error {
	fmt.Println("[INFO] Deleting the secrets for the unhealthy etcd member")
	for _, secret := range secrets {
		name := secret + opts.nodeId
		err := clientset.CoreV1().Secrets("openshift-etcd").Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}
