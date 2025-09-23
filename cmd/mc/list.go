package mc

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type list struct {
	outputFormat string
}

type managementClusterOutput struct {
	Name             string `json:"name" yaml:"name"`
	ID               string `json:"id" yaml:"id"`
	Sector           string `json:"sector" yaml:"sector"`
	Region           string `json:"region" yaml:"region"`
	AccountID        string `json:"account_id" yaml:"account_id"`
	Status           string `json:"status" yaml:"status"`
	Hive             string `json:"hive" yaml:"hive"`
	ProvisionShardID string `json:"provision_shard_id" yaml:"provision_shard_id"`
}

func newCmdList() *cobra.Command {
	l := &list{}
	listCmd := &cobra.Command{
		Use:     "list",
		Short:   "List ROSA HCP Management Clusters",
		Long:    "List ROSA HCP Management Clusters.",
		Example: "osdctl mc list",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			l.outputFormat = cmd.Flag("output").Value.String()
			return l.Run()
		},
	}

	flagSet := listCmd.Flags()
	flagSet.StringVar(
		&l.outputFormat,
		"output",
		"table",
		"Output format. Supported output formats include: table, text, json, yaml",
	)
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

	var output []managementClusterOutput
	provisionShards, err := getProvisionShards(ocm)
	if err != nil {
		log.Printf("Warning: %s", err)
	}

	for _, mc := range managementClusters.Items().Slice() {
		clusterClient := ocm.ClustersMgmt().V1().Clusters().Cluster(mc.ClusterManagementReference().ClusterId())
		clusterResp, err := clusterClient.Get().Send()
		if err != nil {
			log.Printf("failed to find clusters_mgmt cluster for %s: %v", mc.Name(), err)
			continue
		}
		cluster := clusterResp.Body()

		awsAccountID := "NON-STS"
		supportRole := cluster.AWS().STS().SupportRoleARN()
		if supportRole != "" {
			supportRoleARN, err := arn.Parse(supportRole)
			if err != nil {
				log.Printf("failed to convert %s to an ARN: %v", supportRole, err)
			}
			awsAccountID = supportRoleARN.AccountID
		}

		hiveShardResp, err := clusterClient.ProvisionShard().Get().Send()
		if err != nil {
			log.Printf("Could not get provision shard info")
		}
		hiveLink := hiveShardResp.Body().HiveConfig().Server()
		hiveName, _ := getClusterNameFromServerURL(hiveLink)

		serviceClusterName := mc.Parent().Name()

		mcData := managementClusterOutput{
			Name:      mc.Name(),
			ID:        mc.ClusterManagementReference().ClusterId(),
			Sector:    mc.Sector(),
			Region:    mc.Region(),
			AccountID: awsAccountID,
			Status:    mc.Status(),
			Hive:      hiveName,
		}

		if provisionShards != nil {
			ps, ok := provisionShards[serviceClusterName]
			if ok {
				mcData.ProvisionShardID = ps.ID()
			} else {
				mcData.ProvisionShardID = "N/A"
			}
		}

		output = append(output, mcData)
	}

	switch l.outputFormat {
	case "json":
		jsonOutput, err := json.MarshalIndent(output, "", " ")
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
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for i, item := range output {
			fmt.Fprintf(w, "Management Cluster #%d:\n", i+1)
			fmt.Fprintf(w, " Name:\t%s\n", item.Name)
			fmt.Fprintf(w, " ID:\t%s\n", item.ID)
			fmt.Fprintf(w, " Sector:\t%s\n", item.Sector)
			fmt.Fprintf(w, " Region:\t%s\n", item.Region)
			fmt.Fprintf(w, " Account ID:\t%s\n", item.AccountID)
			fmt.Fprintf(w, " Status:\t%s\n", item.Status)
			fmt.Fprintf(w, " Hive:\t%s\n", item.Hive)
			fmt.Fprintf(w, " Provision Shard ID:\t%s\n", item.ProvisionShardID)
			if i < len(output)-1 {
				_, err := fmt.Fprintln(w, "")
				if err != nil {
					return fmt.Errorf("failed to format text output: %v", err)
				}
			}
			err = w.Flush()
			if err != nil {
				return fmt.Errorf("failed to format text output: %v", err)
			}
		}
	case "table":
		w := tabwriter.NewWriter(os.Stdout, 1, 1, 2, ' ', 0)
		_, err := fmt.Fprintln(w, "NAME\tID\tSECTOR\tREGION\tACCOUNT_ID\tSTATUS\tHIVE\tPROVISION_SHARD_ID")
		if err != nil {
			return fmt.Errorf("failed to format table output: %v", err)
		}
		for _, item := range output {
			_, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				item.Name,
				item.ID,
				item.Sector,
				item.Region,
				item.AccountID,
				item.Status,
				item.Hive,
				item.ProvisionShardID,
			)
			if err != nil {
				return fmt.Errorf("failed to format table output: %v", err)
			}
		}
		err = w.Flush()
		if err != nil {
			return fmt.Errorf("failed to format table output: %v", err)
		}
	default:
		return fmt.Errorf("unsupported output format: %s, must be one of: table, text, json, yaml", l.outputFormat)
	}

	return nil
}

func getProvisionShards(ocmClient *ocmsdk.Connection) (map[string]*cmv1.ProvisionShard, error) {
	provisionShardResponse, err := ocmClient.ClustersMgmt().V1().ProvisionShards().List().Send()
	if err != nil {
		return nil, fmt.Errorf("unable to get provision shards: %w", err)
	}

	return processServiceClusters(provisionShardResponse.Items().Slice()), nil
}

func processServiceClusters(shards []*cmv1.ProvisionShard) map[string]*cmv1.ProvisionShard {
	provisionShards := map[string]*cmv1.ProvisionShard{}

	for _, ps := range shards {
		if ps.HypershiftConfig() != nil {
			if strings.Contains(ps.HypershiftConfig().Server(), "hs-sc-") {
				name, _ := getClusterNameFromServerURL(ps.HypershiftConfig().Server())
				provisionShards[name] = ps
			}
		}
	}

	return provisionShards
}

func getClusterNameFromServerURL(server string) (string, error) {
	nameSlice := strings.Split(server, ".")
	if len(nameSlice) < 2 {
		return "", fmt.Errorf("invalid Server URL")
	}
	return nameSlice[1], nil
}
