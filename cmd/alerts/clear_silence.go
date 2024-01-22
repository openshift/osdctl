package alerts

import (
	"fmt"
	"log"
	"github.com/spf13/cobra"
)

func NewCmdClearSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "clear-silence <cluster-id> <silence-id>",
		Short:             "clear all silence",
		Long:              `clear all created silence`,
		Args:              cobra.ExactArgs(2),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ClearSilence(args[0],args[1])
		},
	}
}

//osdctl alerts clear-silence ${CLUSTERID} ${silenceID} 
func ClearSilence(clusterID string, silenceID string){

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	cmd3 := []string{"amtool","silence","expire",silenceID,"--alertmanager.url",LocalHostUrl}

	output, err := GetAlerts(kubeconfig, clientset, LocalHostUrl, cmd3, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("%v", output)
}
