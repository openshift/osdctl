package alerts

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

func NewCmdAddSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "add-silence <cluster-id>",
		Short:             "add a new silence",
		Long:              `add new silence`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			AddSilence(args[0])
		},
	}
}

var (
	alertName = "alert01"
	comment = "some info"
)

//osdctl alerts add-silence ${CLUSTERID} 
func AddSilence(clusterID string){

	_, kubeconfig, clientset, err := GetKubeCli(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := getSilence(kubeconfig, clientset, LocalHostUrl, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Print information from created silence", output)
}

func getSilence(kubeconfig *rest.Config, clientset *kubernetes.Clientset, LocalHostUrl string, PodName string) (string, error) {

	cmd := []string{
		"amtool",
		"silence",
		"add",
		"alertname",
		alertName,
		"--alertmanager.url",
		LocalHostUrl,
		"--comment",
		comment,
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(PodName).
		Namespace(AccountNamespace).SubResource("exec")
	option := &corev1.PodExecOptions{
		Container: ContainerName,
		Command:   cmd,
		Stdin:     false, //changed to false
		Stdout:    true,
		Stderr:    true,
		TTY:       false, 
	}

	if os.Stdin == nil {
		option.Stdin = true
	}
	req.VersionedParams(option, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(kubeconfig, "POST", req.URL())
	if err != nil {
		return "", err
	}

	capture := &logCapture{}
	errorCapture := &logCapture{}

	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  nil, //bytes.NewReader([]byte{}), //changed to nil
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



