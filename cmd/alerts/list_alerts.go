package alerts

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	"github.com/openshift/backplane-cli/pkg/cli/config"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AccountNamespace string = "openshift-monitoring"
	Alertprom string = "alertmanager-main"
	ContainerName string = "alertmanager"
	LocalHostUrl string = "http://localhost:9093"
	PodName string = "alertmanager-main-0"
)

var levelCmd string

type logCapture struct {
	buffer bytes.Buffer
}

type alertCmd struct {
	clusterID string
	getLevel  string
	//active    bool
}

func (capture *logCapture) GetStdOut() string {
	return capture.buffer.String()
}

func (capture *logCapture) Write(p []byte) (n int, err error) {
	a := string(p)
	capture.buffer.WriteString(a)
	return len(p), nil
}

//osdctl alerts list ${CLUSTERID} --level [warning, critical, firing, pending, all] --active bool 

func NewCmdListAlerts() *cobra.Command {
	alertCmd := &alertCmd{}
	newCmd := &cobra.Command{
		Use:               "list <cluster-id>",
		Short:             "list alerts",
		Long:              `Checks the alerts for the cluster`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			alertCmd.clusterID = args[0]
			ListCheck(alertCmd)
		},
	}

	newCmd.Flags().StringVarP(&alertCmd.getLevel, "level", "", "", "Alert level [warning, critical, firing, pending, all]")
	//newCmd.Flags().BoolVar(&alertCmd.active, "active", false, "Show only active alerts")

	return newCmd
}

func getLevel(level string) string{
	switch level{
	case "warning":
		levelCmd = "warning"
	case "critical":
		levelCmd = "critical"
	case "firing":
		levelCmd = "firing"
	case "pending":
		levelCmd = "pending"
	case "all":
		levelCmd = "all" 
	default:
		log.Fatalf("Invalid alert level: %s\n", level)
		return ""
	} 
	return levelCmd
}

func ListCheck(cmd *alertCmd) {
	clusterID := cmd.clusterID
	levelCmd := cmd.getLevel
	//active := cmd.active

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	_, kubeconfig, clientset, err := GetKubeCli(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := getAlerts(kubeconfig, clientset, LocalHostUrl, levelCmd, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Println("Print information from all alert", output)

}

func GetKubeCli(clusterID string) (client.Client, *rest.Config, *kubernetes.Clientset, error) {

	scheme := runtime.NewScheme()
	err := routev1.AddToScheme(scheme)
	if err != nil {
		fmt.Print("failed to register scheme")
	}

	bp, err := config.GetBackplaneConfiguration()
	if err != nil {
		log.Fatalf("failed to load backplane-cli config: %v", err)
	}

	kubeconfig, err := login.GetRestConfig(bp, clusterID)
	if err != nil {
		log.Fatalf("failed to load backplane admin: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("failed to create clientset : %v", err)
	}

	kubeCli, err := client.New(kubeconfig, client.Options{})
	if err != nil {
		log.Fatalf("failed to load kubecli : %v", err)
	}

	return kubeCli, kubeconfig, clientset, err
}

func getAlerts(kubeconfig *rest.Config, clientset *kubernetes.Clientset, LocalHostUrl string, levelCmd string, PodName string) (string, error) {

	cmd := []string{
		"amtool",
		"--alertmanager.url",
		LocalHostUrl,
		"alert",
		levelCmd,
	}
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
		Stdin:  nil,
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

