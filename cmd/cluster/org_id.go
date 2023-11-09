package cluster

import (
	"encoding/json"
	"fmt"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type OrgId struct {
}

type OrgIdOutput struct {
	ExternalId string `json:"external_id"`
	InternalId string `json:"internal_id"`
}

func newCmdOrgId() *cobra.Command {
	o := &OrgId{}

	orgIdCmd := &cobra.Command{
		Use:   "orgId CLUSTER_ID",
		Short: "Get the OCM org ID for a given cluster",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := o.Run(args[0]); err != nil {
				return fmt.Errorf("error fetching OCM org ID for cluster %v", args[0])
			}
			return nil
		},
	}
	return orgIdCmd
}

func (o *OrgId) Run(clusterID string) error {
	if err := ctlutil.IsValidClusterKey(clusterID); err != nil {
		return err
	}

	connection, err := ctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	org, err := ctlutil.GetOrganization(connection, clusterID)
	if err != nil {
		return err
	}

	output, _ := json.MarshalIndent(OrgIdOutput{
		ExternalId: org.ExternalID(),
		InternalId: org.ID(),
	}, "", "  ")

	fmt.Println(string(output))

	return nil
}
