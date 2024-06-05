package silence

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/openshift/osdctl/cmd/alerts/utils"
	kubeutils "github.com/openshift/osdctl/cmd/common"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type addSilenceCmd struct {
	clusterID string
	alertID   []string
	duration  string
	comment   string
	all       bool
	reason    string
}

func NewCmdAddSilence() *cobra.Command {
	addSilenceCmd := &addSilenceCmd{}
	cmd := &cobra.Command{
		Use:               "add <cluster-id> [--all --duration --comment | --alertname --duration --comment]",
		Short:             "Add new silence for alert",
		Long:              `add new silence for specfic or all alert with comment and duration of alert`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			addSilenceCmd.clusterID = args[0]
			AddSilence(addSilenceCmd)
		},
	}

	cmd.Flags().StringSliceVar(&addSilenceCmd.alertID, "alertname", []string{}, "alertname (comma-separated)")
	cmd.Flags().StringVarP(&addSilenceCmd.comment, "comment", "c", "Adding silence using the osdctl alert command", "add comment about silence")
	cmd.Flags().StringVarP(&addSilenceCmd.duration, "duration", "d", "15d", "Adding duration for silence as 15 days") //default duration set to 15 days
	cmd.Flags().BoolVarP(&addSilenceCmd.all, "all", "a", false, "Adding silences for all alert")

	return cmd
}

func AddSilence(cmd *addSilenceCmd) {
	clusterID := cmd.clusterID
	alertID := cmd.alertID
	comment := cmd.comment
	duration := cmd.duration
	all := cmd.all

	username, clustername := GetUserAndClusterInfo(clusterID)

	elevationReasons := []string{
		cmd.reason,
		"Add alert silence via osdctl",
	}

	_, kubeconfig, clientset, err := common.GetKubeConfigAndClient(clusterID, elevationReasons...)
	if err != nil {
		log.Fatal(err)
	}

	if all {
		AddAllSilence(clusterID, duration, comment, username, clustername, kubeconfig, clientset)
	} else if len(alertID) > 0 {
		AddAlertNameSilence(alertID, duration, comment, username, kubeconfig, clientset)
	} else {
		fmt.Println("No valid option specified. Use --all or --alertname.")
	}

}
func AddAllSilence(clusterID, duration, comment, username, clustername string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
	alerts := fetchAllAlerts(clusterID, kubeconfig, clientset)
	for _, alert := range alerts {
		addCmd := []string{
			"amtool",
			"silence",
			"add",
			"alertname=" + alert.Labels.Alertname,
			"--alertmanager.url=" + utils.LocalHostUrl,
			"--duration=" + duration,
			"--comment=" + comment,
		}

		output, err := utils.ExecInPod(kubeconfig, clientset, addCmd)
		if err != nil {
			log.Fatal("Exiting the program")
			return
		}

		formattedOutput := strings.Replace(output, "\n", " ", -1)

		fmt.Printf("Alert %s has been silenced with id \"%s\" for a duration of %s by user \"%s\" \n", alert.Labels.Alertname, formattedOutput, duration, username)
	}
}

func fetchAllAlerts(clusterID string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) []utils.Alert {
	var fetchedAlerts []utils.Alert

	listAlertCmd := []string{"amtool", "--alertmanager.url", utils.LocalHostUrl, "alert", "-o", "json"}
	output, err := utils.ExecInPod(kubeconfig, clientset, listAlertCmd)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal([]byte(output), &fetchedAlerts)
	if err != nil {
		log.Fatal("Error in unmarshaling the alerts", err)
	}

	return fetchedAlerts
}

/*
func AddAllSilence(clusterID, duration, comment, username, clustername string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
	addCmd := []string{
		"amtool",
		"silence",
		"add",
		//"cluster=" + clusterID,
		"namespace=~" + AccountNamespace,
		"--alertmanager.url=" + LocalHostUrl,
		"--duration=" + duration,
		"--comment=" + comment,
	}

	output, err := ExecInPod(kubeconfig, clientset, addCmd)
	if err != nil {
		log.Fatal("Exiting the program")
		return
	}

	formattedOutput := strings.Replace(output, "\n", " ", -1)

	fmt.Printf("All alerts for cluster %s has been silenced with id \"%s\" for a duration of %s by user \"%s\" \n", clustername, formattedOutput, duration, username)
}*/

func AddAlertNameSilence(alertID []string, duration, comment, username string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) {
	for _, alertname := range alertID {
		addCmd := []string{
			"amtool",
			"silence",
			"add",
			"alertname=" + alertname,
			"--alertmanager.url=" + utils.LocalHostUrl,
			"--duration=" + duration,
			"--comment=" + comment,
		}

		output, err := utils.ExecInPod(kubeconfig, clientset, addCmd)
		if err != nil {
			log.Fatal("Exiting the program")
			return
		}

		formattedOutput := strings.Replace(output, "\n", " ", -1)

		fmt.Printf("Alert %s has been silenced with id \"%s\" for duration of %s by user \"%s\" \n", alertname, formattedOutput, duration, username)
	}
}

// Get User name and clustername
func GetUserAndClusterInfo(clusterid string) (string, string) {
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		fmt.Printf("Error %s in create connection.", err)
	}
	//defer connection.Close()
	defer func() {
		if cerr := connection.Close(); cerr != nil {
			fmt.Println("Error closing connection:", cerr)
		}
	}()

	cluster, err := ocmutils.GetCluster(connection, clusterid)
	if err != nil {
		fmt.Printf("Error %s in getting cluster.", err)
	}

	clustername := cluster.Name()

	account, err := connection.AccountsMgmt().V1().CurrentAccount().Get().Send()
	if err != nil {
		fmt.Printf("Error %s in retreving the details of account.", err)
	}

	name, _ := account.Body().GetUsername()
	return name, clustername
}
