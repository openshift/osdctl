package alerts

import (
	"fmt"
	"log"
	"github.com/spf13/cobra"
)

type addSilenceCmd struct {
	clusterID string
	alertID []string
	comment string
}

// add ocm whomai, call the user and print who created this alert 
func NewCmdAddSilence() *cobra.Command {
	addSilenceCmd := &addSilenceCmd{}
	cmd := &cobra.Command{
		Use:   "add-silence <cluster-id> --alertname --comment",
		Short: "Add a new silence for alert present in the cluster",
		Long:  `add new silence for existing alert along with comment and duration of alert`,
		Args:  cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			addSilenceCmd.clusterID = args[0]
			AddSilence(addSilenceCmd)
		},
	}
	
	cmd.Flags().StringSliceVar(&addSilenceCmd.alertID, "alertname", []string{}, "alertname (comma-separated)")
	cmd.Flags().StringVarP(&addSilenceCmd.comment, "comment","c","","add comment about alertname")
		
	return cmd
}

func AddSilence(cmd *addSilenceCmd) {
	clusterID := cmd.clusterID
	alertID := cmd.alertID
	comment := cmd.comment

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	for _, alertname := range alertID {
		addCmd := []string{
			"amtool",
			"silence",
			"add",
			"alertname=" + alertname,
			"--alertmanager.url=" + LocalHostUrl,
			"--comment=" + comment,
		}

		output, err := ExecInPod(kubeconfig, clientset, addCmd)
		if err != nil {
			fmt.Println(err)
		}

		fmt.Printf("Alert %s has been silenced with id:%v\n", alertname, output)
	}
}