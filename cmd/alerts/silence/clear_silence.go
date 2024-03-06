package silence

import (
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

type silenceCmd struct {
	clusterID string
	silenceID []string
	all       bool
}

func NewCmdClearSilence() *cobra.Command {
	silenceCmd := &silenceCmd{}
	cmd := &cobra.Command{
		Use:               "clear <cluster-id> [--all | --silenceID <silence-id>]",
		Short:             "clear all silence",
		Long:              `clear all created silence`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			//silenceCmd.clusterID = args[0]
			//silenceID := args[1 : len(args)-1]
			//ClearSilence(silenceCmd.clusterID, silenceCmd.silenceID)
			silenceCmd.clusterID = args[0]
			ClearSilence(silenceCmd)
		},
	}
	cmd.Flags().StringSliceVar(&silenceCmd.silenceID, "silenceID", []string{}, "silence id (comma-separated)")
	//cmd.Flags().BoolP("all", "a", false, "clear all silences")
	cmd.Flags().BoolVar(&silenceCmd.all, "all", false, "clear all silences")
	return cmd
}

// osdctl alerts clear-silence ${CLUSTERID} --silenceID/--all
func ClearSilence(cmd *silenceCmd) {
	var newID string 
	clusterID := cmd.clusterID
	silenceID := cmd.silenceID
	all := cmd.all

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}
	
	for _, silenceId := range silenceID {
		if all {
			clearCmd := []string{
				"amtool",
				"silence",
				"expire",
				newID,
				"--alertmanager.url=" + LocalHostUrl,
			}

			output, err := ExecInPod(kubeconfig, clientset, LocalHostUrl, clearCmd, PodName)

			if err != nil {
				log.Printf("Error expiring all silences: %v\n", err)
				continue
			}

			fmt.Println("All silences expired successfully.")
			fmt.Printf("amtool output: %v\n", output)

		} else {
			// Assuming SilenceID is a string, replace with the actual field from your Silence struct
			clearCmd := []string{
				"amtool",
				"silence",
				"expire",
				silenceId,
				"--alertmanager.url=" + LocalHostUrl,
			}

			output, err := ExecInPod(kubeconfig, clientset, LocalHostUrl, clearCmd, PodName)

			if err != nil {
				log.Printf("Error expiring silence ID %s: %v\n", silenceId, err)
				continue
			}

			fmt.Printf("Silence ID %s expired successfully.\n", silenceId)
			fmt.Printf("amtool output: %v\n", output)
		}
	}
}
