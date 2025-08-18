package cluster

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/backplane-cli/pkg/ocm"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type getEnvVarsOptions struct {
	ClusterID string
	output    string
}

// formattedOutputCluster contains selected fields from a *cmv1.Cluster object
// plus namespace references for HCP clusters. Although the object is unexported,
// its fields are exported for JSON Marshalling.
type formattedOutputCluster struct {
	Name                   string `json:"name"`
	ID                     string `json:"id"`
	ExternalID             string `json:"external_id"`
	HCPNamespace           string `json:"hcp_namespace,omitempty"`
	KlusterletNamespace    string `json:"klusterlet_namespace,omitempty"`
	HostedClusterNamespace string `json:"hosted_cluster_namespace,omitempty"`
	HiveNamespace          string `json:"hive_namespace,omitempty"`
}

func newCmdGetEnvVars() *cobra.Command {
	opts := newGetEnvVarsOptions()
	getEnvVarsCmd := &cobra.Command{
		Use:               "get-env-vars --cluster-id <cluster-identifier>",
		Short:             "Print a cluster's ID/management namespaces, optionally as env variables",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	getEnvVarsCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "c", "", "Provide internal ID of the cluster")
	_ = getEnvVarsCmd.MarkFlagRequired("cluster-id")

	getEnvVarsCmd.Flags().StringVarP(&opts.output, "output", "o", "text", "Valid formats are ['text', 'json', 'env']")

	return getEnvVarsCmd
}

func (o *getEnvVarsOptions) run() error {
	omcClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if err := omcClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	cluster, err := utils.GetCluster(omcClient, o.ClusterID)
	if err != nil {
		return err
	}

	foCluster, err := newFormattedOutputCluster(cluster)
	if err != nil {
		return err
	}

	switch o.output {
	case "text":
		fmt.Print(foCluster)
	case "json":
		fmt.Print(foCluster.json())
	case "env":
		fmt.Print(foCluster.env())
	default:
		return fmt.Errorf("unknown output format %q. Supported formats are \"text\", \"json\", \"env\"", o.output)
	}

	return nil
}

func newGetEnvVarsOptions() *getEnvVarsOptions {
	return &getEnvVarsOptions{}
}

func newFormattedOutputCluster(cluster *cmv1.Cluster) (formattedOutputCluster, error) {
	foCluster := formattedOutputCluster{
		Name:       cluster.Name(),
		ID:         cluster.ID(),
		ExternalID: cluster.ExternalID(),
	}

	if !cluster.Hypershift().Enabled() {
		env, err := ocm.DefaultOCMInterface.GetOCMEnvironment()
		if err != nil {
			return formattedOutputCluster{}, err
		}
		envName := env.Name()
		foCluster.HiveNamespace = fmt.Sprintf("uhc-%s-%s", envName, cluster.ID())
		return foCluster, nil
	}

	hcpNS, err := utils.GetHCPNamespace(cluster.ID())
	if err != nil {
		return formattedOutputCluster{}, err
	}
	hostedClusterNS := strings.SplitAfter(hcpNS, cluster.ID())[0]
	klusterletNS := fmt.Sprintf("klusterlet-%s", cluster.ID())

	foCluster.HCPNamespace = hcpNS
	foCluster.HostedClusterNamespace = hostedClusterNS
	foCluster.KlusterletNamespace = klusterletNS

	return foCluster, nil
}

func (f formattedOutputCluster) String() string {
	var buf strings.Builder
	printer := printer.NewTablePrinter(&buf, 20, 1, 3, ' ')

	printer.AddRow([]string{"Name:", f.Name})
	printer.AddRow([]string{"ID:", f.ID})
	printer.AddRow([]string{"External ID:", f.ExternalID})

	if f.HCPNamespace != "" {
		printer.AddRow([]string{"HCP namespace:", f.HCPNamespace})
	}

	if f.HostedClusterNamespace != "" {
		printer.AddRow([]string{"HC namespace:", f.HostedClusterNamespace})
	}

	if f.KlusterletNamespace != "" {
		printer.AddRow([]string{"Klusterlet namespace:", f.KlusterletNamespace})
	}

	if f.HiveNamespace != "" {
		printer.AddRow([]string{"Hive namespace:", f.HiveNamespace})
	}

	printer.Flush()

	return buf.String()
}

func (f formattedOutputCluster) json() string {
	if buf, err := json.Marshal(f); err == nil {
		return string(buf)
	} else {
		fmt.Fprintf(os.Stderr, "Cannot marshal JSON: invalid input: %v\n", err)
		return ""
	}
}

func (f formattedOutputCluster) env() string {
	var b strings.Builder

	printExport(&b, "CLUSTER_NAME", f.Name)
	printExport(&b, "CLUSTER_ID", f.ID)
	printExport(&b, "CLUSTER_UUID", f.ExternalID)

	if f.HCPNamespace != "" {
		printExport(&b, "HCP_NAMESPACE", f.HCPNamespace)
	}

	if f.HostedClusterNamespace != "" {
		printExport(&b, "HC_NAMESPACE", f.HostedClusterNamespace)
	}

	if f.KlusterletNamespace != "" {
		printExport(&b, "KLUSTERLET_NAMESPACE", f.KlusterletNamespace)
	}

	if f.HiveNamespace != "" {
		printExport(&b, "HIVE_NAMESPACE", f.HiveNamespace)
	}

	return b.String()
}

func printExport(w io.Writer, key, value string) {
	fmt.Fprintf(w, "export %s=%s\n", key, value)
}
