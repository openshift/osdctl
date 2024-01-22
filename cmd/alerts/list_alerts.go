package alerts

import (
	"encoding/json"
	"fmt"
	"log"
	"github.com/spf13/cobra"
)

var (
	levelCmd string
	cmdStatus []string
)


type alertCmd struct {
	clusterID	string
	alertLevel	string
	active	bool
}

type alertJson struct{
	Labels struct {
		Alertname string `json:"alertname"`
		Severity string `json:"severity"`
	}`json:"labels"`

	Status struct {
		State string `json:"state"`
	}`json:"status"`

	Annotations struct{
		Summary string `json:"summary"`
	}`json:"annotations"`
}

//osdctl alerts list ${CLUSTERID} --level [warning, critical, firing, pending, all] --active bool 
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
	newCmd.Flags().BoolVar(&alertCmd.active, "active", false, "Show only active alerts")

	return newCmd
}

func alertLevel(level string) string{
	switch level{
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

	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	clusterID := cmd.clusterID
	levelcmd := cmd.alertLevel
	active := cmd.active

	cmd1 := []string{"amtool","--alertmanager.url",LocalHostUrl,"alert","-o","json"}
	
	//Show all active alerts
	cmd_active := []string{"amtool","--alertmanager.url",LocalHostUrl,"alert","query","-a"}
	//Show unprocessed alerts
	//cmd0 := []string{"amtool","--alertmanager.url",utils.LocalHostUrl,"alert","-o","extended"}

	if active{
		cmdStatus = cmd_active
	}

	kubeconfig, clientset, err := GetKubeConfigClient(clusterID)
	if err != nil {
		log.Fatal(err)
	}

	output, err := GetAlerts(kubeconfig, clientset,LocalHostUrl, cmd1,PodName)
	if err != nil {
		fmt.Println(err)
	}

	outputSlice := []byte(output)

	var alerts []alertJson
	var foundAlert bool = false

	err = json.Unmarshal(outputSlice, &alerts)
	if err != nil {
		fmt.Println("Error in unmarshal:", err)
		return
	}
	
	for _, a := range alerts {
		labels, status, annotations := a.Labels, a.Status, a.Annotations
		if levelcmd == labels.Severity{
			fmt.Printf("AlertName:%s\t Severity:%s\t State:%s\t Message:%s\n",
				labels.Alertname,
				labels.Severity,
				status.State,
				annotations.Summary)
			foundAlert = true
			break
		}
	}

	if !foundAlert {
		fmt.Printf("No such Alert found with requested severity %s\n", levelcmd)
	}

}

