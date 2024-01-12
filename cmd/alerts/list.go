package alerts

import (
	"context"
	"fmt"
	"github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	"github.com/openshift/backplane-cli/pkg/cli/config"
	v1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/spf13/cobra"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"log"
	"sigs.k8s.io/controller-runtime/pkg/client"
	routev1 "github.com/openshift/api/route/v1"
	"strings"
)


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
	var Namespace string = "openshift-monitoring"
	var rulename string

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	kubeCli, kubeClient, err := getKubeCli(clusterID)
	if err != nil{
		log.Fatal(err)
	}

	var promRuleList v1.PrometheusRuleList
	if err := kubeCli.List(context.TODO(), &promRuleList, &client.ListOptions{Namespace: Namespace}); err != nil {
		log.Fatal(err)
	}
	
	var alertmanager v1.Alertmanager
		if err := kubeCli.List(context.TODO(), &alertmanager, &client.ListOptions{Namespace: Namespace}); err != nil {
			log.Fatal(err)
	}

	//fmt.Print("PrometheusRules in openshift-monitoring namespace:\n")
	for _, promRule := range promRuleList.Items {
		//fmt.Printf("Name: %s\n", promRule.Name)
		if promRule.Name == "alertmanager-main-rules"{
			rulename = strings.TrimSuffix(promRule.Name, "-rules")
			fmt.Printf("Rule Name found: %s\n", rulename)
		}
	}
	
	//fmt.Printf("AlertManager %v\n", alertmanager.Spec.ClusterAdvertiseAddress)
	
	//routenew := routev1.Route{}
	//kubeClient.RESTClient().Get()
	// Get Route information
	/* Fetch Dynakube CRD
	var dynakubeCRD map[string]interface{}
	data, err := clientset.RESTClient().
		Get().
		AbsPath("/apis/dynatrace.com/v1beta1").
		Namespace("dynatrace").
		Resource("dynakubes").
		DoRaw(context.TODO())*/

	// Get route information
    routeList, err := kubeClient.RouteV1().Routes(Namespace).List(context.TODO(), meta_v1.ListOptions{
        LabelSelector: fmt.Sprintf("app=%s", rulename),
    })
    if err != nil {
        log.Fatalf("Error getting route information for %s in namespace %s: %v", rulename, Namespace, err)
    }

	if len(routeList.Items) == 0 {
		log.Fatalf("No route found for %s in namespace %s", rulename, Namespace)
	}

	route := routeList.Items[0]
	fmt.Printf("Route information for %s in namespace %s: Host is %s.\n", rulename, Namespace, route.Spec.Host)
	
	// Get Ingress information
	ingressList, err := kubeClient.NetworkingV1().Ingresses(Namespace).List(context.TODO(), meta_v1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", rulename),
	})
	if err != nil {
		log.Fatalf("Error getting Ingress information for %s in namespace %s: %v", rulename, Namespace, err)
	}

	if len(ingressList.Items) == 0 {
		log.Fatalf("No Ingress found for %s in namespace %s", rulename, Namespace)
	}

	ingress := ingressList.Items[0] 
	fmt.Printf("Ingress information for %s in namespace %s: Host is %s.\n", rulename, Namespace, ingress.Spec.Rules[0].Host)

	//Get service information 
	svc, err := kubeClient.CoreV1().Services(Namespace).Get(context.TODO(), rulename, meta_v1.GetOptions{})
	if err != nil {
		log.Fatalf("Error getting route %s in namespace %s: %v", rulename, Namespace, err)
	}
	fmt.Printf("Route information for %s in namespace %s for host is %s.", rulename, Namespace, svc.Spec.Host)
}

func getKubeCli(clusterID string) (client.Client, *kubernetes.Clientset, error) {

	scheme := runtime.NewScheme()
	err := v1.AddToScheme(scheme)
	err = routev1.AddToScheme(scheme) // added to scheme

	if err != nil {
		fmt.Print("failed to register scheme")
	}

	bp, err := config.GetBackplaneConfiguration()
	if err != nil {
		log.Fatalf("failed to load backplane-cli config: %v", err)
	}

	kubeconfig, err := login.GetRestConfigAsUser(bp, clusterID, "backplane-cluster-admin")
	if err != nil {
		log.Fatalf("failed to load backplane admin: %v", err)
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		log.Fatalf("failed to load clientset : %v", err)
	}

	kubeCli, err := client.New(kubeconfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("failed to load kubecli: %v", err)
	}

	return kubeCli, clientset, err
}

