package silence

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type silenceCmd struct {
	clusterID string
	silenceID []string
	all       bool
}

func NewCmdClearSilence() *cobra.Command {
	silenceCmd := &silenceCmd{}
	cmd := &cobra.Command{
		Use:               "expire <cluster-id> [--all | --silence-id <silence-id>]",
		Short:             "Expire Silence for alert",
		Long:              `expire all silence or based on silenceid`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			silenceCmd.clusterID = args[0]
			ClearSilence(silenceCmd)
		},
	}
	cmd.Flags().StringSliceVar(&silenceCmd.silenceID, "silence-id", []string{}, "silence id (comma-separated)")
	cmd.Flags().BoolVarP(&silenceCmd.all, "all", "a", false, "clear all silences")
	return cmd
}

func ClearSilence(cmd *silenceCmd) {
	clusterID := cmd.clusterID
	silenceID := cmd.silenceID
	all := cmd.all

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	if all {
		ClearAllSilence(kubeconfig, clientset)
	} else {
		if len(silenceID) > 0 {
			ClearSilenceByID(silenceID, kubeconfig, clientset)
		} else {
			fmt.Println("No valid option specified. Using a default option")
			ClearAllSilence(kubeconfig, clientset)
		}
	}
}

func ClearAllSilence(kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
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
		return
	}

	finalOutput := strings.Fields(formattedOutput)
	countsilence := len(finalOutput)
	for _, silence := range finalOutput {
		clearCmd := []string{
			"amtool",
			"silence",
			"expire",
			silence,
			"--alertmanager.url=" + LocalHostUrl,
		}

		countsilence = countsilence - 1

		_, err := ExecInPod(kubeconfig, clientset, clearCmd)

		if err != nil {
			log.Printf("Error expiring silence ID \"%s\" : %v\n", silence, err)
			return
		}

		fmt.Printf("SilenceID \"%s\" expired successfully.\n", silence)

		if countsilence == 0 {
			fmt.Println()
			fmt.Printf("All SilenceID expired successfully.\n")
			return
		}
	}
}

func ClearSilenceByID(silenceID []string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
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
			log.Printf("Error expiring silence ID \"%s\" %v\n", silenceId, err)
			continue
		}

		fmt.Printf("Requested SilenceID \"%s\" expired successfully.\n", silenceId)
	}
}
