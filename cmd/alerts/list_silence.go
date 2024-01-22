package alerts

import (
	"fmt"
	"log"
	"github.com/spf13/cobra"
)

func NewCmdListSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "list-silence <cluster-id>",
		Short:             "list all silence",
		Long:              `list all  silence`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ListSilence(args[0])
		},
	}
}

//osdctl alerts list-silence ${CLUSTERID} 
func ListSilence(clusterID string){
	cmd4 := []string{"amtool","silence","--alertmanager.url", LocalHostUrl}

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := GetAlerts(kubeconfig, clientset, LocalHostUrl, cmd4, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("%v", output)
}
