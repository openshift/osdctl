package cluster

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	EtcdNamespaceName       = "openshift-etcd"
	EtcdMemberConditionType = "EtcdMembersAvailable"
	ContainerName           = "etcdctl"
	MasterNodeLabel         = "node-role.kubernetes.io/master"
	EtcdPodMatchLabelName   = "k8s-app"
	EtcdPodMatchValueName   = "etcd"
	EtcdLabelSelector       = "k8s-app=etcd"
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

func newCmdEtcdHealthCheck() *cobra.Command {
	return &cobra.Command{
		Use:               "etcd-health-check <cluster-id>",
		Short:             "Checks the etcd components and member health",
		Long:              `Checks etcd component health status for member replacement`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(EtcdHealthCheck(args[0]))
		},
	}
}

func EtcdHealthCheck(clusterId string) error {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	kubeCli, kconfig, clientset, err := common.GetKubeConfigAndClient(clusterId)
	if err != nil {
		return err
	}

	err = ControlplaneNodeStatus(kubeCli)
	if err != nil {
		return err
	}

	podlist, err := EtcdPodStatus(kubeCli)
	if err != nil {
		return err
	}

	unhealthyMember, err := EtcdCrStatus(kubeCli)
	if err != nil {
		return err
	}

	for _, v := range etcdctlCmd {
		output, err := Etcdctlhealth(kconfig, clientset, v, podlist.Items[0].Name)
		if err != nil {
			fmt.Println(err)
		}
		fmt.Printf("$ %s\n", v)
		fmt.Println(output)
	}

	if unhealthyMember != "" {
		fmt.Printf("[INFO] %s is unhealthy.\nRun \"osdctl cluster etcd-member-replace %s --node %s\" to replace the member \n", unhealthyMember, clusterId, unhealthyMember)
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

func EtcdCrStatus(kubeCli client.Client) (string, error) {

	etcdCR := &operatorv1.Etcd{}
	if err := kubeCli.Get(context.TODO(), client.ObjectKey{
		Name: "cluster",
	}, etcdCR); err != nil {
		return "", err
	}

	etcdConditionList := etcdCR.Status.Conditions
	var message string
	for _, v := range etcdConditionList {
		if v.Type == EtcdMemberConditionType {
			fmt.Println("+---------------------------------------------------------------+")
			fmt.Println("|               ETCD MEMBER HEALTH STATUS                       |")
			fmt.Println("+---------------------------------------------------------------+")
			fmt.Printf("%s\n\n", v.Message)
			message = v.Message
		}
	}

	// Getting the unhealthy member name
	list := strings.Split(message, ",")
	if len(list) == 1 {
		return "", nil
	}
	message = strings.Split(strings.TrimSpace(list[1]), " ")[0]
	return message, nil
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
		Container: ContainerName,
		Command:   cmd,
		Stdin:     true,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
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
		Stdin:  bytes.NewReader([]byte{}),
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
