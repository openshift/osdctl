package mc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type list struct {
	// Add a reference to the output format
	outputFormat string
}

// Define a struct for output formatting
type managementClusterOutput struct {
	Name      string `json:"name" yaml:"name"`
	ID        string `json:"id" yaml:"id"`
	Sector    string `json:"sector" yaml:"sector"`
	Region    string `json:"region" yaml:"region"`
	AccountID string `json:"account_id" yaml:"account_id"`
	Status    string `json:"status" yaml:"status"`
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
			// Get the global output flag value
			l.outputFormat = cmd.Flag("output").Value.String()
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

	// Prepare a slice to hold the structured output
	output := []managementClusterOutput{}

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
			} else {
				awsAccountID = supportRoleARN.AccountID
			}
		}

		// Add the cluster info to our output slice
		output = append(output, managementClusterOutput{
			Name:      mc.Name(),
			ID:        mc.ClusterManagementReference().ClusterId(),
			Sector:    mc.Sector(),
			Region:    mc.Region(),
			AccountID: awsAccountID,
			Status:    mc.Status(),
		})
	}

	// Handle output based on the format flag
	switch l.outputFormat {
	case "json":
		jsonOutput, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to format JSON output: %v", err)
		}
		fmt.Println(string(jsonOutput))
	case "yaml":
		yamlOutput, err := yaml.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to format YAML output: %v", err)
		}
		fmt.Println(string(yamlOutput))
	case "text":
		// Plain text output format, one cluster per section with labeled fields
		for i, item := range output {
			if i > 0 {
				fmt.Println() // Add blank line between clusters
			}
			fmt.Printf("Management Cluster #%d:\n", i+1)
			fmt.Printf("  Name:       %s\n", item.Name)
			fmt.Printf("  ID:         %s\n", item.ID)
			fmt.Printf("  Sector:     %s\n", item.Sector)
			fmt.Printf("  Region:     %s\n", item.Region)
			fmt.Printf("  Account ID: %s\n", item.AccountID)
			fmt.Printf("  Status:     %s\n", item.Status)
		}
	default:
		// Default tabular output
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 1, ' ', 0)
		fmt.Fprintln(w, "NAME\tID\tSECTOR\tREGION\tACCOUNT_ID\tSTATUS")
		for _, item := range output {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Name,
				item.ID,
				item.Sector,
				item.Region,
				item.AccountID,
				item.Status,
			)
		}
		w.Flush()
	}

	return nil
}
