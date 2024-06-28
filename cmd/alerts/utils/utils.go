package utils

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
	PrimaryPod       = "alertmanager-main-0"
	SecondaryPod     = "alertmanager-main-1"
)

// ExecInAlertManagerPod attempts to execute the given command in the primary Alertmanager pod,
// and if that fails, it retries with the secondary Alertmanager pod.
func ExecInAlertManagerPod(kubeconfig *rest.Config, clientset *kubernetes.Clientset, cmd []string) (string, error) {
	var cmdOutput string
	var err error

	cmdOutput, err = ExecWithPod(kubeconfig, clientset, PrimaryPod, AccountNamespace, cmd)
	if err == nil {
		return cmdOutput, err
	}

	cmdOutput, err = ExecWithPod(kubeconfig, clientset, SecondaryPod, AccountNamespace, cmd)
	if err == nil {
		return cmdOutput, err
	}

	return "", fmt.Errorf("exec failed, please put silence manually: %w", err)
}

func ExecWithPod(kubeconfig *rest.Config, clientset *kubernetes.Clientset, podName string, AccountNamespace string, cmd []string) (string, error) {
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).
		Namespace(AccountNamespace).SubResource("exec")
	option := &corev1.PodExecOptions{
		Container: ContainerName,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}

	req.VersionedParams(option, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(kubeconfig, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("failed to create executor: %w", err)
	}

	capture := &cluster.LogCapture{}
	errorCapture := &cluster.LogCapture{}
	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: capture,
		Stderr: errorCapture,
		Tty:    false,
	})
	if err != nil {
		return "", fmt.Errorf("failed to stream with context: %w", err)
	}

	return capture.GetStdOut(), nil
}
