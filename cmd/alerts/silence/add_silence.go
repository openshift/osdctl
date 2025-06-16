package silence

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/openshift/osdctl/cmd/alerts/utils"
	"github.com/openshift/osdctl/cmd/common"
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
		Use:               "add --cluster-id <cluster-identifier> [--all --duration --comment | --alertname --duration --comment]",
		Short:             "Add new silence for alert",
		Long:              `add new silence for specfic or all alert with comment and duration of alert`,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			AddSilence(addSilenceCmd)

		},
	}

	cmd.Flags().StringVar(&addSilenceCmd.clusterID, "cluster-id", "", "Provide the internal ID of the cluster")
	cmd.Flags().StringSliceVar(&addSilenceCmd.alertID, "alertname", []string{}, "alertname (comma-separated)")
	cmd.Flags().StringVarP(&addSilenceCmd.comment, "comment", "c", "Adding silence using the osdctl alert command", "add comment about silence")
	cmd.Flags().StringVarP(&addSilenceCmd.duration, "duration", "d", "15d", "Adding duration for silence as 15 days") //default duration set to 15 days
	cmd.Flags().BoolVarP(&addSilenceCmd.all, "all", "a", false, "Adding silences for all alert")
	cmd.Flags().StringVar(&addSilenceCmd.reason, "reason", "", "The reason for this command, which requires elevation, to be run (usualy an OHSS or PD ticket)")

	_ = cmd.MarkFlagRequired("cluster-id")
	_ = cmd.MarkFlagRequired("reason")

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
		err := AddAllSilence(clusterID, duration, comment, username, clustername, kubeconfig, clientset)
		if err != nil {
			fmt.Printf("Failed to add silence: %s", err)
		}
	} else if len(alertID) > 0 {
		err := AddAlertNameSilence(alertID, duration, comment, username, kubeconfig, clientset)
		if err != nil {
			fmt.Printf("Failed to add silence: %s", err)
		}
	} else {
		fmt.Println("No valid option specified. Use --all or --alertname.")
	}
}

func AddAllSilence(clusterID, duration, comment, username, clustername string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) error {
	alerts := fetchAllAlerts(kubeconfig, clientset)
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

		output, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, addCmd)
		if err != nil {
			log.Fatal("Exiting the program")
			return fmt.Errorf("Failed to exec in AlertManager pod: %w", err)
		}

		formattedOutput := strings.Replace(output, "\n", "", -1)

		fmt.Printf("Alert %s has been silenced with id \"%s\" for a duration of %s by user \"%s\" \n", alert.Labels.Alertname, formattedOutput, duration, username)
	}

	return nil
}

func fetchAllAlerts(kubeconfig *rest.Config, clientset *kubernetes.Clientset) []utils.Alert {
	var fetchedAlerts []utils.Alert

	listAlertCmd := []string{"amtool", "--alertmanager.url", utils.LocalHostUrl, "alert", "-o", "json"}
	output, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, listAlertCmd)
	if err != nil {
		log.Fatal(err)
	}

	err = json.Unmarshal([]byte(output), &fetchedAlerts)
	if err != nil {
		log.Fatal("Error in unmarshaling the alerts", err)
	}

	return fetchedAlerts
}

func AddAlertNameSilence(alertID []string, duration, comment, username string, kubeconfig *rest.Config, clientset *kubernetes.Clientset) error {
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

		output, err := utils.ExecInAlertManagerPod(kubeconfig, clientset, addCmd)
		if err != nil {
			return fmt.Errorf("failed to exec in AlertManager pod: %w", err)
		}

		formattedOutput := strings.Replace(output, "\n", "", -1)

		fmt.Printf("Alert %s has been silenced with id \"%s\" for duration of %s by user \"%s\" \n", alertname, formattedOutput, duration, username)
	}

	return nil
}

// Get User name and clustername
func GetUserAndClusterInfo(clusterid string) (string, string) {
	connection, err := ocmutils.CreateConnection()
	if err != nil {
		fmt.Printf("Error %s in create connection.", err)
	}

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
