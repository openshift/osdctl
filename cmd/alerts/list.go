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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var alerturl string = "http://localhost:9093"
//const ContainerName string = "prometheus"
const ContainerName string = "alertmanager"
var accountNamespace string = "openshift-monitoring"
var alertprom string = "alertmanager-main"
//var PodName string = "prometheus-k8s-0"
var PodName string = "alertmanager-main-0"

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

func NewCmdList() *cobra.Command {
	return &cobra.Command{
		Use:               "list <cluster-id>",
		Short:             "list alerts",
		Long:              `Checks the alerts for the cluster`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ListCheck(args[0])
		},
	}
}

func ListCheck(clusterID string) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	kClient, kubeconfig, clientset, err := getKubeCli(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	err = routev1.AddToScheme(kClient.Scheme())
	if err != nil {
		fmt.Println("Could not add route scheme")
		return
	}

	route := routev1.Route{}
	err = kClient.Get(context.TODO(), types.NamespacedName{
		Namespace: accountNamespace,
		Name:      alertprom,
	}, &route)
	if err != nil {
		fmt.Println("Could not retrieve desired alertmanager-main route.")
		return
	}

	fmt.Printf("Retrieved route to host: %s\n", route.Spec.Host)
	/*header := "http://"
	posturl := "/api/v1/alerts"
	routeurl := route.Spec.Host
	url := header + routeurl + posturl
	fmt.Printf("Retrieved route to host after changes passed to getAlerts func: %s\n", url)
`*/
	output, err := getAlerts(kubeconfig, clientset, alerturl, PodName)
	if err != nil {
		fmt.Println(err)
	}
	//fmt.Printf("$ %s\n", containerCmd)
	fmt.Println("Print information from all alert", output)

}

func getKubeCli(clusterID string) (client.Client, *rest.Config, *kubernetes.Clientset, error) {

	scheme := runtime.NewScheme()
	err := routev1.AddToScheme(scheme) // added to scheme
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

	// create the clientset
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

func getAlerts(kubeconfig *rest.Config, clientset *kubernetes.Clientset, alerturlurl string, PodName string) (string, error) {

	cmd := []string{
		"amtool",
		"--alertmanager.url",
		alerturl,
		"alert",
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(PodName).
		Namespace(accountNamespace).SubResource("exec")
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

