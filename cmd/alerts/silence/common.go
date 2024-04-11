package silence

import (
	"context"
	"fmt"

	"github.com/openshift/osdctl/cmd/cluster"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	AccountNamespace = "openshift-monitoring"
	ContainerName    = "alertmanager"
	LocalHostUrl     = "http://localhost:9093"
	PodName          = "alertmanager-main-0"
)

// ExecInPod is designed to execute a command inside a Kubernetes pod and capture its output.
func ExecInPod(kubeconfig *rest.Config, clientset *kubernetes.Clientset, cmd []string) (string, error) {
	// request is constructed using the Kubernetes clientset to interact with the Kubernetes API.
	// It specifies that the request is for executing a command inside a pod (SubResource("exec")).
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(PodName).
		Namespace(AccountNamespace).SubResource("exec")
	//various options for executing the command inside the pod
	option := &corev1.PodExecOptions{
		Container: ContainerName,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}

	req.VersionedParams(option, scheme.ParameterCodec)
	//SPDY executor is created using the Kubernetes configuration and the request URL.
	//This executor will be responsible for executing the command inside the pod.

	exec, err := remotecommand.NewSPDYExecutor(kubeconfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	capture := &cluster.LogCapture{}
	errorCapture := &cluster.LogCapture{}
	//capture and errorCapture are instances of cluster.LogCapture,
	//which are used to capture the stdout and stderr output of the executed command.

	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: capture,
		Stderr: errorCapture,
		Tty:    false,
	})

	if err != nil {
		return "", fmt.Errorf("failed to stream with context: %w", err)
	}

	cmdOutput := capture.GetStdOut()
	return cmdOutput, nil
}
