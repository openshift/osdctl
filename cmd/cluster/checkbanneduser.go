package cluster

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

const BanCodeExportControlCompliance = "export_control_compliance"

func newCmdCheckBannedUser() *cobra.Command {
	clusterID := ""
	cmd := &cobra.Command{
		Use:               "check-banned-user --cluster-id <cluster-identifier>",
		Short:             "Checks if the cluster owner is a banned user.",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(CheckBannedUser(clusterID))
		},
	}

	cmd.Flags().StringVarP(&clusterID, "cluster-id", "c", "", "Provide internal ID of the cluster")
	_ = cmd.MarkFlagRequired("cluster-id")

	return cmd
}

func CheckBannedUser(clusterID string) error {
	ocm, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if ocmCloseErr := ocm.Close(); ocmCloseErr != nil {
			fmt.Printf("Cannot close the ocm (possible memory leak): %q", ocmCloseErr)
		}
	}()

	fmt.Print("Finding subscription account: ")
	subscription, err := utils.GetSubscription(ocm, clusterID)
	if err != nil {
		return err
	}
	if status := subscription.Status(); status != "Active" {
		return fmt.Errorf("Expecting status 'Active' found %v\n", status)
	}
	fmt.Printf("Account %v - %v - %v\n", subscription.SupportLevel(), subscription.Creator().HREF(), subscription.Status())

	fmt.Print("Finding account owner: ")
	creator, err := utils.GetAccount(ocm, subscription.Creator().ID())
	if err != nil {
		return err
	}

	userEmail := creator.Email()
	userBanned := creator.Banned()
	userBanCode := creator.BanCode()
	userBanDescription := creator.BanDescription()
	lastUpdate := creator.UpdatedAt()

	fmt.Printf("%v\n-------------------\nLast Update : %v\n", userEmail, lastUpdate)
	if userBanned {
		fmt.Println("User is banned")
		fmt.Printf("Ban code = %v\n", userBanCode)
		fmt.Printf("Ban description = %v\n", userBanDescription)
		if userBanCode == BanCodeExportControlCompliance {
			fmt.Println("User banned due to export control compliance.\nPlease follow the steps detailed here: https://github.com/openshift/ops-sop/blob/master/v4/alerts/UpgradeConfigSyncFailureOver4HrSRE.md#user-banneddisabled-due-to-export-control-compliance .")
			return nil
		}
		return nil
	}
	fmt.Println("User allowed")
	return nil
}
