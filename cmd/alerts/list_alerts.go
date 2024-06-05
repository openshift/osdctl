package alerts

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openshift/osdctl/cmd/alerts/utils"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/spf13/cobra"
)

// alertCmd represnts information associated with cluster and level.
type alertCmd struct {
	clusterID  string
	alertLevel string
}

// NewCmdListAlerts implements the list alert functionality.
func NewCmdListAlerts() *cobra.Command {
	alertCmd := &alertCmd{}
	newCmd := &cobra.Command{
		Use:               "list <cluster-id> --level [warning, critical, firing, pending, all]",
		Short:             "List all alerts or based on severity",
		Long:              `Checks the alerts for the cluster and print the list based on severity`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			alertCmd.clusterID = args[0]
			ListAlerts(alertCmd)
		},
	}

	newCmd.Flags().StringVarP(&alertCmd.alertLevel, "level", "l", "all", "Alert level [warning, critical, firing, pending, all]")

	return newCmd
}

// ListAlerts provides alerts based on input severity.
func ListAlerts(cmd *alertCmd) {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	clusterID := cmd.clusterID
	alertLevel := cmd.alertLevel

	if alertLevel == "" {
		log.Printf("No alert level specified. Defaulting to 'all'")
		getAlertLevel(clusterID, "all")
	} else if alertLevel == "warning" || alertLevel == "critical" || alertLevel == "firing" || alertLevel == "pending" || alertLevel == "info" || alertLevel == "none" || alertLevel == "all" {
		getAlertLevel(clusterID, alertLevel)
	} else {
		fmt.Printf("Invalid alert level \"%s\" \n", alertLevel)
		return
	}
}

func getAlertLevel(clusterID, alertLevel string){
	var alerts []utils.Alert

	listAlertCmd := []string{"amtool", "--alertmanager.url", utils.LocalHostUrl, "alert", "-o", "json"}

	_, kubeconfig, clientset, err := common.GetKubeConfigAndClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := utils.ExecInPod(kubeconfig, clientset, listAlertCmd)
	if err != nil {
		fmt.Println(err)
	}

	outputSlice := []byte(output)

	err = json.Unmarshal(outputSlice, &alerts)
	if err != nil {
		fmt.Println("Error in unmarshaling the labels", err)
		return
	}

	err = json.Unmarshal(outputSlice, &alerts)
	if err != nil {
		fmt.Println("Error in unmarshaling the labels", err)
		return
	}

	foundAlert := false
	fmt.Printf("Alert Information:\n")
	for _, alert := range alerts {
		if alertLevel == "" || alertLevel == alert.Labels.Severity || alertLevel == "all" {
			labels, status, annotations := alert.Labels, alert.Status, alert.Annotations
			printAlert(labels, annotations, status)
			foundAlert = true
		}
	}

	if !foundAlert {
		fmt.Printf("No such Alert found with requested \"%s\" severity.\n", alertLevel)
	}
}

func printAlert(labels utils.AlertLabels, annotations utils.AlertAnnotations, status utils.AlertStatus){
	fmt.Printf("  AlertName:  %s\n", labels.Alertname)
	fmt.Printf("  Severity:   %s\n", labels.Severity)
	fmt.Printf("  State:      %s\n", status.State)
	fmt.Printf("  Message:    %s\n", annotations.Summary)
}
