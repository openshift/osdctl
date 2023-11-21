package mc

import (
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type list struct {
}

func newCmdList() *cobra.Command {
	l := &list{}

	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List ROSA HCP Management Clusters",
		Long:    "List ROSA HCP Management Clusters",
		Example: "osdctl mc list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return l.Run()
		},
	}

	return listCmd
}

func (l *list) Run() error {
	ocm, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocm.Close()

	managementClusters, err := ocm.OSDFleetMgmt().V1().ManagementClusters().List().Send()
	if err != nil {
		return fmt.Errorf("failed to list management clusters: %v", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
	fmt.Fprintln(w, "NAME\tID\tSECTOR\tREGION\tACCOUNT_ID\tSTATUS")
	for _, mc := range managementClusters.Items().Slice() {
		cluster, err := ocm.ClustersMgmt().V1().Clusters().Cluster(mc.ClusterManagementReference().ClusterId()).Get().Send()
		if err != nil {
			log.Printf("failed to find clusters_mgmt cluster for %s: %v", mc.Name(), err)
			continue
		}

		awsAccountID := "NON-STS"
		supportRole := cluster.Body().AWS().STS().SupportRoleARN()
		if supportRole != "" {
			supportRoleARN, err := arn.Parse(supportRole)
			if err != nil {
				log.Printf("failed to convert %s to an ARN: %v", supportRole, err)
			}
			awsAccountID = supportRoleARN.AccountID
		}

		fmt.Fprintln(w, mc.Name()+"\t"+
			mc.ClusterManagementReference().ClusterId()+"\t"+
			mc.Sector()+"\t"+
			mc.Region()+"\t"+
			awsAccountID+"\t"+
			mc.Status())
	}
	w.Flush()

	return nil
}
