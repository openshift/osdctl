package alerts

import (
	"fmt"
	"log"
	"strings"
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
		Use:               "clear-silence <cluster-id> [--all | --silenceID <silence-id>]",
		Short:             "Clear Silence for alert",
		Long:              `clear all silence based on silenceid`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			silenceCmd.clusterID = args[0]
			ClearSilence(silenceCmd)
		},
	}
	cmd.Flags().StringSliceVar(&silenceCmd.silenceID, "silenceID", []string{}, "silence id (comma-separated)")
	cmd.Flags().BoolVar(&silenceCmd.all, "all", true, "clear all silences")
	return cmd
}

// osdctl alerts clear-silence ${CLUSTERID} --silenceID/--all
func ClearSilence(cmd *silenceCmd) {
	clusterID := cmd.clusterID
	silenceID := cmd.silenceID
	all := cmd.all

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	queryCmd := []string{
		"amtool",
		"silence",
		"query",
		"-q",
		"--alertmanager.url=" + LocalHostUrl,
	}
	queryOutput, err := ExecInPod(kubeconfig, clientset, queryCmd)
	
	if err != nil {
		fmt.Print("some issue in query command")
	}
	
	formattedOutput := strings.Join(strings.Fields(queryOutput), " ")
	
	if formattedOutput == " " || formattedOutput == "" {
		fmt.Println("No Silence has been set for alerts, please create new silence")
	} else if all {
		finalOutput := strings.Fields(formattedOutput)
		for _, silence := range finalOutput {
			clearCmd := []string{
				"amtool",
				"silence",
				"expire",
				silence,
				"--alertmanager.url=" + LocalHostUrl,
			}
	
			_, err := ExecInPod(kubeconfig, clientset, clearCmd)
	
			if err != nil {
				log.Printf("Error expiring silence ID %s: %v\n", silence, err)
				return
			}
	
			fmt.Printf("All Silence ID : %s expired successfully.\n", silence)
		}
	
		//fmt.Println("All Silences expired successfully for alerts")
	
	} else {
		for _, silenceId := range silenceID {
			clearCmd := []string{
				"amtool",
				"silence",
				"expire",
				silenceId,
				"--alertmanager.url=" + LocalHostUrl,
			}
	
			_, err := ExecInPod(kubeconfig, clientset, clearCmd)
	
			if err != nil {
				log.Printf("Error expiring silence ID: %s: %v\n", silenceId, err)
				continue
			}
	
			fmt.Printf("Requested SilenceID : %s expired successfully.\n", silenceId)
		}
	}
}	