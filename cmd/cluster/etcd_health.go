package cluster

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EtcdNamespaceName       = "openshift-etcd"
	EtcdMemberConditionType = "EtcdMembersAvailable"
	ConatinerName           = "etcdctl"
	MasterNodeLabel         = "node-role.kubernetes.io/master"
	EtcdPodMatchLabelName   = "k8s-app"
	EtcdPodMatchValueName   = "etcd"
)

var etcdctlCmd = []string{
	"etcdctl endpoint status -w table",
	"etcdctl endpoint health -w table",
	"etcdctl member list -w table",
}

type logCapture struct {
	buffer bytes.Buffer
}

func (capture *logCapture) GetStdOut() string {
	return capture.buffer.String()
}

func (capture *logCapture) Write(p []byte) (n int, err error) {
	a := string(p)
	capture.buffer.WriteString(a)
	return len(p), nil
}

func newCmdEtcdHealthCheck(kubeCli client.Client, flags *genericclioptions.ConfigFlags) *cobra.Command {
	return &cobra.Command{
		Use:               "etcd-health-check",
		Short:             "Checks the etcd components and member health",
		Long:              `Checks etcd component health status for member replacement`,
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(EtcdHealthCheck(kubeCli))
		},
	}
}

func EtcdHealthCheck(kubeCli client.Client) error {

	err := ControlplaneNodeStatus(kubeCli)
	if err != nil {
		fmt.Println(err)
	}

	podlist, err := EtcdPodStatus(kubeCli)
	if err != nil {
		fmt.Println(err)
	}

	err = EtcdCrStatus(kubeCli)
	if err != nil {
		fmt.Println(err)
	}

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

	MatchPodName := false
	var etcdPodName string
	for !MatchPodName {
		fmt.Print("Enter Etcd Pod Name: ")
		fmt.Scan(&etcdPodName)
		for _, pods := range podlist.Items {
			if etcdPodName == pods.Name {
				MatchPodName = true
			}
		}
	}

	for _, v := range etcdctlCmd {
		output, err := Etcdctlhealth(kconfig, clientset, v, etcdPodName)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("$ %s\n", v)
		fmt.Println(output)
	}
	return nil
}

func ControlplaneNodeStatus(kubeCli client.Client) error {
	nodeList := &corev1.NodeList{}
	if err := kubeCli.List(context.TODO(), nodeList, client.MatchingLabels{MasterNodeLabel: ""}); err != nil {
		return err
	}

	fmt.Println("+----------------------------------------------------------------+")
	fmt.Println("|                CONTROLPLANE NODE STATUS                        |")
	fmt.Println("+----------------------------------------------------------------+")

	for _, node := range nodeList.Items {
		for _, condition := range node.Status.Conditions {
			if condition.Type == corev1.NodeReady {
				fmt.Printf("%s\t%s\t%s\n", node.Name, corev1.NodeReady, condition.Status)
			}
		}
	}
	return nil
}

func EtcdPodStatus(kubeCli client.Client) (*corev1.PodList, error) {

	pods := &corev1.PodList{}
	if err := kubeCli.List(context.TODO(), pods, client.InNamespace(EtcdNamespaceName), client.MatchingLabels{EtcdPodMatchLabelName: EtcdPodMatchValueName}); err != nil {
		return nil, err
	}

	fmt.Println("+----------------------------------------------------------------+")
	fmt.Println("|               ETCD POD STATUS                                  |")
	fmt.Println("+----------------------------------------------------------------+")

	for _, pods := range pods.Items {
		containerReadyCount := 0
		for _, containers := range pods.Status.ContainerStatuses {
			if containers.Ready {
				containerReadyCount++
			}
		}
		fmt.Printf("%s\t%v/%v\n", pods.Name, containerReadyCount, len(pods.Status.ContainerStatuses))
	}
	return pods, nil
}

func EtcdCrStatus(kubeCli client.Client) error {

	etcdCR := &operatorv1.Etcd{}
	if err := kubeCli.Get(context.TODO(), client.ObjectKey{
		Name: "cluster",
	}, etcdCR); err != nil {
		return err
	}

	etcdConditionList := etcdCR.Status.Conditions
	for _, v := range etcdConditionList {
		if v.Type == EtcdMemberConditionType {
			fmt.Println("+---------------------------------------------------------------+")
			fmt.Println("|               ETCD MEMBER HEALTH STATUS                       |")
			fmt.Println("+---------------------------------------------------------------+")
			fmt.Printf("%s\n\n", v.Message)
		}
	}
	return nil
}

func Etcdctlhealth(kconfig *rest.Config, clientset *kubernetes.Clientset, etcdctlCmd string, etcdPodName string) (string, error) {

	cmd := []string{
		"sh",
		"-c",
		etcdctlCmd,
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(etcdPodName).
		Namespace(EtcdNamespaceName).SubResource("exec")
	option := &corev1.PodExecOptions{
		Container: ConatinerName,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       true,
	}

	if os.Stdin == nil {
		option.Stdin = false
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(kconfig, "POST", req.URL())
	if err != nil {
		return "", err
	}

	capture := &logCapture{}
	errorCapture := &logCapture{}

	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: capture,
		Stderr: errorCapture,
		Tty:    false,
	})

	if err != nil {
		return "", err
	}

	cmdOutput := capture.GetStdOut()
	return cmdOutput, nil
}
