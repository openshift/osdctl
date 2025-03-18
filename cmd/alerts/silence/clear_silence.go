package silence

import (
	"fmt"
	"log"
	"strings"

	"github.com/openshift/osdctl/cmd/alerts/utils"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type silenceCmd struct {
	clusterID  string
	silenceIDs []string
	all        bool
	reason     string
}

func NewCmdClearSilence() *cobra.Command {
	silenceCmd := &silenceCmd{}
	cmd := &cobra.Command{
		Use:               "expire [--cluster-id=<cluster-id>] [--all | --silence-id <silence-id>]",
		Short:             "Expire Silence for alert",
		Long:              `expire all silence or based on silenceid`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			if silenceCmd.clusterID == "" {
				fmt.Println("Error: --cluster-id flag is required")
				_ = cmd.Help()
				return
			}
			ClearSilence(silenceCmd)
		},
	}

	cmd.Flags().StringVar(&silenceCmd.clusterID, "cluster-id", "", "Provide the internal ID of the cluster")
	cmd.Flags().StringSliceVar(&silenceCmd.silenceIDs, "silence-id", []string{}, "silence id (comma-separated)")
	cmd.Flags().BoolVarP(&silenceCmd.all, "all", "a", false, "clear all silences")
	cmd.Flags().StringVar(&silenceCmd.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

	return cmd
}

func ClearSilence(cmd *silenceCmd) {
	clusterID := cmd.clusterID
	silenceIDs := cmd.silenceIDs
	all := cmd.all

	elevationReasons := []string{
		cmd.reason,
		"Clear alertmanager silence for a cluster via osdctl",
	}

	_, kubeconfig, clientset, err := common.GetKubeConfigAndClient(clusterID, elevationReasons...)
	if err != nil {
		log.Fatal(err)
	}

	if all {
		ClearAllSilence(kubeconfig, clientset)
	} else if len(silenceIDs) > 0 {
		ClearSilenceByID(silenceIDs, kubeconfig, clientset)
	} else {
		fmt.Println("No valid option specified. Using a default option to clear all silences")
		ClearAllSilence(kubeconfig, clientset)
	}
}

func ClearAllSilence(kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
	queryCmd := []string{
		"amtool",
		"silence",
		"query",
		"-q",
		"--alertmanager.url=" + utils.LocalHostUrl,
	}

	queryOutput, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, queryCmd)
	if err != nil {
		fmt.Println("Error encountered while expiring all silence:", err)
		return
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
			"--alertmanager.url=" + utils.LocalHostUrl,
		}

		countsilence = countsilence - 1
		_, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, clearCmd)
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

func ClearSilenceByID(silenceIDs []string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
	for _, silenceId := range silenceIDs {
		clearCmd := []string{
			"amtool",
			"silence",
			"expire",
			silenceId,
			"--alertmanager.url=" + utils.LocalHostUrl,
		}

		_, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, clearCmd)
		if err != nil {
			log.Printf("Error expiring silence ID \"%s\" %v\n", silenceId, err)
			continue
		}

		fmt.Printf("Requested SilenceID \"%s\" expired successfully.\n", silenceId)
	}
}
