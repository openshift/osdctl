package silence

import (
	"bytes"
	"context"
	"fmt"
	"github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	"github.com/openshift/backplane-cli/pkg/cli/config"
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

type logCapture struct {
	buffer bytes.Buffer
}

func (capture *logCapture) GetStdOut() string {
	return capture.buffer.String()
}

func (capture *logCapture) Write(p []byte) (n int, err error) {
	a := string(p)
	_, err = capture.buffer.WriteString(a)
	return len(p), err
}

func GetKubeConfigClient(clusterID string) (*rest.Config, *kubernetes.Clientset, error) {

	bp, err := config.GetBackplaneConfiguration()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load backplane-cli config: %w", err)
	}

	kubeconfig, err := login.GetRestConfig(bp, clusterID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load backplane admin: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create clientset : %w", err)
	}

	return kubeconfig, clientset, nil
}

func ExecInPod(kubeconfig *rest.Config, clientset *kubernetes.Clientset, cmd []string) (string, error) {

	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(PodName).
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
		return "", fmt.Errorf("failed to create SPDY executor: %w", err)
	}

	capture := &logCapture{}
	errorCapture := &logCapture{}

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
