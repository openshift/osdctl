package alerts

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/openshift/osdctl/cmd/alerts/silence"
	"github.com/spf13/cobra"
)

// alertCmd represnts information associated with cluster and level.
type alertCmd struct {
	clusterID  string
	alertLevel string
}

// Labels represents a set of labels associated with an alert.
type Labels struct {
	Alertname string `json:"alertname"`
	Severity  string `json:"severity"`
}

// Status represents a set of state associated with an alert.
type Status struct {
	State string `json:"state"`
}

// Annotations represents a set of summary/description associated with an alert.
type Annotations struct {
	Summary string `json:"summary"`
}

// Alert represents a set of above declared struct Labels,Status and annoataions
type Alert struct {
	Labels      Labels      `json:"labels"`
	Status      Status      `json:"status"`
	Annotations Annotations `json:"annotations"`
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
		log.Default().Printf("No alert level specified. Defaulting to 'all'")
		getAlertLevel(clusterID, "all")
	} else if alertLevel == "warning" || alertLevel == "critical" || alertLevel == "firing" || alertLevel == "pending" || alertLevel == "info" || alertLevel == "none" || alertLevel == "all" {
		getAlertLevel(clusterID, alertLevel)
	} else {
		fmt.Printf("Invalid alert level \"%s\" \n", alertLevel)
		return
	}
}

func getAlertLevel(clusterID, alertLevel string) {
	var alerts []Alert

	listAlertCmd := []string{"amtool", "--alertmanager.url", silence.LocalHostUrl, "alert", "-o", "json"}

	kubeconfig, clientset, err := silence.GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := silence.ExecInPod(kubeconfig, clientset, listAlertCmd)
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

func printAlert(labels Labels, annotations Annotations, status Status) {
	fmt.Printf("  AlertName:  %s\n", labels.Alertname)
	fmt.Printf("  Severity:   %s\n", labels.Severity)
	fmt.Printf("  State:      %s\n", status.State)
	fmt.Printf("  Message:    %s\n", annotations.Summary)
}
