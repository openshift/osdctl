package alerts

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

type alertCmd struct {
	clusterID  string
	alertLevel string
}

type Labels struct {
	Alertname string `json:"alertname"`
	Severity  string `json:"severity"`
}

type Status struct {
	State string `json:"state"`
}

type Annotations struct {
	Summary string `json:"summary"`
}

type Alert struct {
	Labels      Labels      `json:"labels"`
	Status      Status      `json:"status"`
	Annotations Annotations `json:"annotations"`
}

func NewCmdListAlerts() *cobra.Command {
	alertCmd := &alertCmd{}
	newCmd := &cobra.Command{
		Use:               "list <cluster-id> --level [warning, critical, firing, pending, all]",
		Short:             "List the alerts based on severity",
		Long:              `Checks the alerts for the cluster and print the list based on severity`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			alertCmd.clusterID = args[0]
			ListAlerts(alertCmd)
		},
	}

	newCmd.Flags().StringVarP(&alertCmd.alertLevel, "level", "l", "", "Alert level [warning, critical, firing, pending, all]")
	return newCmd
}

// osdctl alerts list ${CLUSTERID} --level [warning, critical...]
func ListAlerts(cmd *alertCmd) {
	var alerts []Alert
	var levelCmd string

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	clusterID := cmd.clusterID
	levelcmd := cmd.alertLevel

	if levelcmd == "" {
		fmt.Println("No alert level specified. Defaulting to 'all'.")
		levelcmd = "all"
	} else if levelcmd == "warning" || levelcmd == "critical" || levelcmd == "firing" || levelcmd == "pending" || levelcmd == "info" || levelcmd == "none" || levelcmd == "all" {
		levelCmd = levelcmd
	} else {
		fmt.Printf("Invalid alert level \"%s\" \n", levelcmd)
		return
	}

	ListAlertCmd := []string{"amtool", "--alertmanager.url", LocalHostUrl, "alert", "-o", "json"}

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := ExecInPod(kubeconfig, clientset, ListAlertCmd)
	if err != nil {
		fmt.Println(err)
	}

	outputSlice := []byte(output)

	err = json.Unmarshal(outputSlice, &alerts)
	if err != nil {
		fmt.Println("Error in unmarshaling the labels", err)
		return
	}

	foundAlert := false
	for _, alert := range alerts {
		if levelCmd == "" || levelCmd == alert.Labels.Severity || levelCmd == "all" {
			labels, status, annotations := alert.Labels, alert.Status, alert.Annotations
			PrintAlert(labels, annotations, status)
			foundAlert = true
		}
	}

	if !foundAlert {
		fmt.Printf("No such Alert found with requested \"%s\" severity.\n", levelCmd)
	}
}

func PrintAlert(labels Labels, annotations Annotations, status Status) {
	fmt.Printf("Alert Information:\n")
	fmt.Printf("  AlertName:  %s\n", labels.Alertname)
	fmt.Printf("  Severity:   %s\n", labels.Severity)
	fmt.Printf("  State:      %s\n", status.State)
	fmt.Printf("  Message:    %s\n", annotations.Summary)
}
