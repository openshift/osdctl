package silence

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func NewCmdAddSilence() *cobra.Command {
	return &cobra.Command{
		Use:   "add <cluster-id> <alertname1> <alertname2>... <comment>",
		Short: "add a new silence",
		Long:  `add new silence`,
		Args:  cobra.MinimumNArgs(3),
		Run: func(cmd *cobra.Command, args []string) {
			clusterID := args[0]
			comment := args[len(args)-1]
			alertNames := args[1 : len(args)-1]
			AddSilence(clusterID, alertNames, comment)
		},
	}
}

// osdctl alerts add-silence ${CLUSTERID} ${alertname1} ${alertname2}... ${comment}
func AddSilence(clusterID string, alertNames []string, comment string) {
	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	for _, alertname := range alertNames {
		addCmd := []string{
			"amtool",
			"silence",
			"add",
			"alertname=" + alertname,
			"--alertmanager.url=" + LocalHostUrl,
			"--comment=" + comment,
		}

		output, err := ExecInPod(kubeconfig, clientset, LocalHostUrl, addCmd, PodName)
		if err != nil {
			fmt.Println(err)
		}

		fmt.Printf("%v", output)
	}
}
