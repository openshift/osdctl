package alerts

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

func NewCmdAddSilence() *cobra.Command {
	return &cobra.Command{
		Use:               "add-silence <cluster-id> <alertname> <comment>",
		Short:             "add a new silence",
		Long:              `add new silence`,
		Args:              cobra.ExactArgs(3),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			AddSilence(args[0], args[1], args[2])
		},
	}
}

// osdctl alerts add-silence ${CLUSTERID} ${alertname} ${comment}
func AddSilence(clusterID string, alertName string, comment string) {
	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	cmd2 := []string{
		"amtool",
		"silence",
		"add",
		"alertname=" + alertName,
		"--alertmanager.url=" + LocalHostUrl,
		"--comment=" + comment,
	}

	output, err := ExecInPod(kubeconfig, clientset, LocalHostUrl, cmd2, PodName)
	if err != nil {
		fmt.Println(err)
	}

	fmt.Printf("%v", output)
}
