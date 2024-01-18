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
var SilenceID string = "5c9539c7-4ffe-4105-9121-4ee4f1755fa1"

func NewCmdClearSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "clear-silence <cluster-id>",
		Short:             "clear a already created silence",
		Long:              `add new silence`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ClearSilence(args[0])
		},
	}
}

//osdctl alerts clear-silence ${CLUSTERID} 
func ClearSilence(clusterID string){
	
	_, kubeconfig, clientset, err := GetKubeCli(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := getClearSilence(kubeconfig, clientset, LocalHostUrl, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Print information from cleared silence", output)
}

func getClearSilence(kubeconfig *rest.Config, clientset *kubernetes.Clientset, LocalHostUrl string, PodName string) (string, error) {

	cmd := []string{
		"amtool",
		"silence",
		"expire",
		SilenceID,
		"--alertmanager.url",
		LocalHostUrl,
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



