package cluster

import (
	"encoding/json"
	"fmt"

	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type OrgId struct {
	clusterID string
}

type OrgIdOutput struct {
	ExternalId string `json:"external_id"`
	InternalId string `json:"internal_id"`
}

func newCmdOrgId() *cobra.Command {
	o := &OrgId{}
	orgIdCmd := &cobra.Command{
		Use:   "orgId --cluster-id <cluster-identifier",
		Short: "Get the OCM org ID for a given cluster",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Run(); err != nil {
				return fmt.Errorf("error fetching OCM org ID for cluster %v", o.clusterID)
			}
			return nil
		},
	}

	// Add cluster-id flag
	orgIdCmd.Flags().StringVarP(&o.clusterID, "cluster-id", "c", "", "The internal ID of the cluster to check (required)")
	if err := orgIdCmd.MarkFlagRequired("cluster-id"); err != nil {
		// Handle error in flag configuration
		fmt.Printf("Error marking cluster-id flag as required: %v\n", err)
	}

	return orgIdCmd
}

func (o *OrgId) Run() error {
	if err := ctlutil.IsValidClusterKey(o.clusterID); err != nil {
		return err
	}

	connection, err := ctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	org, err := ctlutil.GetOrganization(connection, o.clusterID)
	if err != nil {
		return err
	}

	output, _ := json.MarshalIndent(OrgIdOutput{
		ExternalId: org.ExternalID(),
		InternalId: org.ID(),
	}, "", " ")
	fmt.Println(string(output))
	return nil
}
