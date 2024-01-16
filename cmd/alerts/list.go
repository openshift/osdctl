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

const ContainerName string = "prometheus"

var accountNamespace string = "openshift-monitoring"
var alertprom string = "alertmanager-main"
var PodName string = "prometheus-k8s-0"

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
	if err != nil{
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
	 		Name: alertprom,
	 	}, &route)
	if err != nil {
	 	fmt.Println("Could not retrieve desired alertmanager-main route.")
	 	return
	}

	/*fmt.Printf("Retrieved route to host: %s\n", route.Spec.Host)
	posturl := "/api/v1/alerts"
	routeurl := route.Spec.Host
	url :=  routeurl + posturl
	fmt.Printf("Retrieved route to host: %s\n", url)*/
	url := "http://localhost:9090/api/v1/alerts"

	output, err := getAlerts(kubeconfig, clientset, PodName, url)
	if err != nil {
		fmt.Println("Could not execute the oc exec command")
		return
   	}
	fmt.Println(output)
    
}

func getKubeCli(clusterID string) (client.Client, *rest.Config , *kubernetes.Clientset, error) {

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

//c.Println(fmt.Sprintf("Use \n\n    oc exec -it --as %s -n %s %s -- /bin/bash\n\nto run commands in the pod. All 'oc' commands run within the pod will be executed against the cluster '%s' (this can be verified by running `oc cluster-info` in the pod)" pod.Namespace, pod.Name)
//oc -n openshift-monitoring exec -c prometheus prometheus-k8s-0 -- curl -s   'http://localhost:9090/api/v1/alerts'

func getAlerts(kubeconfig *rest.Config, clientset *kubernetes.Clientset, PodName string, url string) (string, error) {

	cmd := []string{
		"sh",
		"-c",
		"-- curl",
		"-s",
	}
	req := clientset.CoreV1().RESTClient().Post().Resource("pods").Name(PodName).
		Namespace(accountNamespace).RequestURI(url).SubResource("exec")
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


	exec, err := remotecommand.NewSPDYExecutor(kubeconfig, "POST", req.URL())
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

