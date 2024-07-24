package silence

import (
	"fmt"
	"log"

	"github.com/openshift/osdctl/cmd/common"
	orgutils "github.com/openshift/osdctl/cmd/org"
	ocmutils "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type AddOrgSilenceCmd struct {
	organization string
	alertID      []string
	duration     string
	comment      string
	all          bool
}

func NewCmdAddOrgSilence() *cobra.Command {
	AddOrgSilenceCmd := &AddOrgSilenceCmd{}
	cmd := &cobra.Command{
		Use:               "org <org-id> [--all --duration --comment | --alertname --duration --comment]",
		Short:             "Add new silence for alert for org",
		Long:              `add new silence for specfic or all alerts with comment and duration of alert for an organization. OHSS required for org-wide silence`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			AddOrgSilenceCmd.organization = args[0]
			AddOrgSilence(AddOrgSilenceCmd)
		},
	}

	cmd.Flags().StringSliceVar(&AddOrgSilenceCmd.alertID, "alertname", []string{}, "alertname (comma-separated)")
	cmd.Flags().StringVarP(&AddOrgSilenceCmd.comment, "comment", "c", "", "add comment about silence. OHSS required for org-wide silence")
	cmd.Flags().StringVarP(&AddOrgSilenceCmd.duration, "duration", "d", "15d", "add duration for silence") //default duration set to 15 days
	cmd.Flags().BoolVarP(&AddOrgSilenceCmd.all, "all", "a", false, "add silences for all alert")
	cmd.MarkFlagRequired("comment")

	return cmd
}

// AddOrgSilence adds alert silences to organization's clusters
func AddOrgSilence(cmd *AddOrgSilenceCmd) {
	alertID := cmd.alertID
	comment := cmd.comment
	duration := cmd.duration
	all := cmd.all
	organizationID := cmd.organization

	subscriptions, err := orgutils.SearchSubscriptions(organizationID, orgutils.StatusActive)
	if err != nil {
		log.Fatal(err)
	} else if len(subscriptions) == 0 {
		log.Fatal("No subscriptions found with that organization ID")
	}

	connection, err := ocmutils.CreateConnection()
	if err != nil {
		log.Fatal(err)
	}

	organization, err := ocmutils.GetOrganization(connection, subscriptions[0].ClusterID())
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("Are you sure you want silence alerts for %d clusters for this organization: %s", len(subscriptions), organization.Name())
	ocmutils.ConfirmPrompt()

	for _, subscription := range subscriptions {
		clusterID := subscription.ClusterID()
		if len(clusterID) == 0 {
			log.Printf("Cluster ID invalid, skipping: %s", clusterID)
			continue //Skip invalid clusters
		} else {
			log.Printf("Silencing alert(s) on cluster: %s", clusterID)
		}

		username, clustername := GetUserAndClusterInfo(clusterID)

		_, kubeconfig, clientset, err := common.GetKubeConfigAndClient(clusterID)
		if err != nil {
			log.Print(err)
			continue //Skip if cluster is not in supported state
		}

		if all {
			err := AddAllSilence(clusterID, duration, comment, username, clustername, kubeconfig, clientset)
			if err != nil {
				log.Print(err)
			}
		} else if len(alertID) > 0 {
			err := AddAlertNameSilence(alertID, duration, comment, username, kubeconfig, clientset)
			if err != nil {
				log.Print(err)
			}
		} else {
			fmt.Println("No valid option specified. Use --all or --alertname.")
		}
	}
}
