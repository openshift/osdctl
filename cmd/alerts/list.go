package alerts

import (
	"context"
	"fmt"
	"log"

	routev1 "github.com/openshift/api/route/v1"
	"github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	"github.com/openshift/backplane-cli/pkg/cli/config"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"k8s.io/apimachinery/pkg/types"
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

	var accountNamespace string = "openshift-monitoring"
	var alertprom string = "alertmanager-main"

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	kClient, _, err := getKubeCli(clusterID)
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
	fmt.Printf("Retrieved route to host: %s\n", route.Spec.Host)
    
}

func getKubeCli(clusterID string) (client.Client, *rest.Config , error) {

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

	kubeCli, err := client.New(kubeconfig, client.Options{})
	if err != nil {
		log.Fatalf("failed to load kubecli : %v", err)
	}

	return kubeCli, kubeconfig, err
}

