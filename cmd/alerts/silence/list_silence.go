package silence

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openshift/osdctl/cmd/alerts/utils"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/spf13/cobra"
)

type listSilenceCmd struct {
	clusterID string
	reason    string
}

func NewCmdListSilence() *cobra.Command {
	listSilenceCmd := &listSilenceCmd{}
	cmd := &cobra.Command{
		Use:               "list --cluster-id <cluster-identifier>",
		Short:             "List all silences",
		Long:              `print the list of silences`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ListSilence(listSilenceCmd)
		},
	}
	cmd.Flags().StringVar(&listSilenceCmd.clusterID, "cluster-id", "", "Provide the internal ID of the cluster")
	cmd.Flags().StringVar(&listSilenceCmd.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")
	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")
	return cmd
}

func ListSilence(cmd *listSilenceCmd) {
	var silences []utils.Silence
	silenceCmd := []string{"amtool", "silence", "--alertmanager.url", utils.LocalHostUrl, "-o", "json"}
	elevationReasons := []string{
		cmd.reason,
		"Clear alertmanager silence for a cluster via osdctl",
	}

	_, kubeconfig, clientset, err := common.GetKubeConfigAndClient(cmd.clusterID, elevationReasons...)
	if err != nil {
		log.Fatal(err)
	}

	op, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, silenceCmd)
	if err != nil {
		fmt.Println("Error encountered while listing the silences:", err)
		return
	}

	opSlice := []byte(op)
	err = json.Unmarshal(opSlice, &silences)
	if err != nil {
		fmt.Println("Error in unmarshaling the data", err)
	}

	fmt.Printf("Silence Information:\n")
	if len(silences) > 0 {
		for _, silence := range silences {
			printSilence(silence)
		}
	} else {
		fmt.Println("No silences found, all silence has been cleared.")
	}
}

func printSilence(silence utils.Silence) {
	id, matchers, status, created, starts, end, comment := silence.ID, silence.Matchers, silence.Status, silence.CreatedBy, silence.StartsAt, silence.EndsAt, silence.Comment
	fmt.Println("-------------------------------------------")
	fmt.Printf("SilenceID: %s\n", id)
	fmt.Printf("Status: %s\n", status.State)
	fmt.Printf("Created By: %s\n", created)
	fmt.Printf("Starts At: %s\n", starts)
	fmt.Printf("Ends At: %s\n", end)
	fmt.Printf("Comment: %s\n", comment)
	fmt.Println("Matchers:")
	for _, matcher := range matchers {
		fmt.Printf("  %s: %s\n", matcher.Name, matcher.Value)
	}
	fmt.Println("-------------------------------------------")
}
