package alerts

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/spf13/cobra"
)

var levelCmd string

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

// osdctl alerts list ${CLUSTERID} --level [warning, critical, firing, pending, all] --active bool
func NewCmdListAlerts() *cobra.Command {
	alertCmd := &alertCmd{}
	newCmd := &cobra.Command{
		Use:               "list <cluster-id>",
		Short:             "list alerts",
		Long:              `Checks the alerts for the cluster`,
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

func alertLevel(level string) string {
	switch level {
	case "warning":
		levelCmd = "warning"
	case "critical":
		levelCmd = "critical"
	case "firing":
		levelCmd = "firing"
	case "pending":
		levelCmd = "pending"
	case "all":
		levelCmd = "all"
	default:
		log.Fatalf("Invalid alert level: %s\n", level)
		return ""
	}
	return levelCmd
}

func ListAlerts(cmd *alertCmd) {
	var alerts []Alert

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	clusterID := cmd.clusterID
	levelcmd := cmd.alertLevel

	ListAlertCmd := []string{"amtool", "--alertmanager.url", LocalHostUrl, "alert", "-o", "json"}

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := ExecInPod(kubeconfig, clientset, LocalHostUrl, ListAlertCmd, PodName)
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
		if levelcmd == "" || levelcmd == alert.Labels.Severity || levelcmd == "all" {
			labels, status, annotations := alert.Labels, alert.Status, alert.Annotations
			fmt.Printf("AlertName:%s\t Severity:%s\t State:%s\t Message:%s\n",
				labels.Alertname,
				labels.Severity,
				status.State,
				annotations.Summary)
			foundAlert = true
		}
	}

	if !foundAlert {
		fmt.Printf("No such Alert found with requested %s severity.\n", levelcmd)
	}

}
