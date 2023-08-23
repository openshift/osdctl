package cluster

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type flagOptions struct {
	nodeId string
}

func newCmdEtcdMemberReplacement(kubeCli client.Client, flags *genericclioptions.ConfigFlags) *cobra.Command {
	opts := &flagOptions{}
	replaceCmd := &cobra.Command{
		Use:               "etcd-member-replacement <node-id>",
		Short:             "Replaces an unhealthy etcd node",
		Long:              `Replaces an unhealthy ectd node using the member id provided`,
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(EtcdReplaceMember(kubeCli, *opts))
		},
	}
	replaceCmd.Flags().StringVar(&opts.nodeId, "node", "", "Node ID")
	return replaceCmd
}

func EtcdReplaceMember(kubeCli client.Client, opts flagOptions) error {
	// TODO : 1. Identify the unhealthy members
	// TODO : 2. Identify the cause -> CrashLoopBackOff/ Machine Not Running/ Node Not Ready
	// TODO : 3. For each of these, perform the replacement

	// CrashLoopBackOff replacement

	// Machine Not Running replacement

	// Node Not Ready replacement
	var kubeconfig *string

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	kconfig, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(kconfig)
	if err != nil {
		panic(err.Error())
	}

	namespace := "openshift-etcd"
	listOptions := metav1.ListOptions{
		LabelSelector: "k8s-app=etcd",
	}

	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	if err != nil {
		return err
	}
	for _, pod := range pods.Items {
		for _, c := range pod.Status.ContainerStatuses {
			if c.State.Waiting != nil && (c.State.Waiting.Reason == "CrashLoopBackOff" || c.State.Waiting.Reason == "Error") {
				err := ReplaceCrashBackLoopOff(kconfig, clientset, pod.ObjectMeta.Name, opts)
				if err != nil {
					return err
				}
				return nil
			}
		}
	}
	return fmt.Errorf("the etcd member seems to be healthy. Please run health-check again")
}

func ReplaceCrashBackLoopOff(kconfig *rest.Config, client *kubernetes.Clientset, pod string, opts flagOptions) error {
	podName := "etcd-" + opts.nodeId
	if podName != pod {
		return fmt.Errorf("the etcd member seems to be healthy. Please run health-check again")
	}
	cmd := "etcdctl member list -w table | grep " + opts.nodeId + " | awk '{ print $2 }'"
	memberId, _ := Etcdctlhealth(kconfig, client, cmd, pod)
	memberId = strings.TrimSpace(memberId)
	fmt.Printf("Replacing pod %s having member id %s ", pod, memberId)
	///

	// prompt := utils.ConfirmPrompt()
	// fmt.Printf("%T\n", prompt)
	// if !prompt {
	// 	return fmt.Errorf("operation cancelled by user")
	// }
	fmt.Print("Continue? (y/N): ")

	var response string
	_, err := fmt.Scanln(&response) // Erroneous input will be handled by the default case below
	if err != nil {
		fmt.Println(err)
	}
	switch strings.ToLower(response) {
	case "y", "yes":
		remove_cmd := "etcdctl member remove " + memberId
		output, err := Etcdctlhealth(kconfig, client, remove_cmd, pod)
		if err != nil {
			fmt.Println("Could not replace pod. Refer error below")
			return err
		}
		fmt.Println(output)
		return nil
	case "n", "no":
		return fmt.Errorf("operation cancelled by user")
	default:
		fmt.Println("Invalid input. Expecting (y)es or (N)o")
		return fmt.Errorf("operation cancelled by user")
	}

	///
	// remove_cmd := "etcdctl member remove " + memberId
	// output, err := Etcdctlhealth(kconfig, client, remove_cmd, pod)
	// if err != nil {
	// 	fmt.Println("Could not replace pod. Refer error below")
	// 	return err
	// }
	// fmt.Println(output)
	// return nil
}
