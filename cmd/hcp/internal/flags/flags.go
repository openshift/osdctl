package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const clusterDescription = "Internal ID, name, or external ID of the HCP cluster"

// Cluster is an HCP cluster identifier shared by HCP commands.
type Cluster string

// AddRequired binds the required cluster identifier flag.
func (f *Cluster) AddRequired(cmd *cobra.Command) {
	f.Add(cmd.Flags())
	f.MarkRequired(cmd)
}

// Add binds the cluster identifier flag.
func (f *Cluster) Add(flags *pflag.FlagSet) {
	flags.StringVarP((*string)(f), "cluster-id", "C", string(*f), clusterDescription)
}

// MarkRequired marks the cluster identifier flag as required.
func (f *Cluster) MarkRequired(cmd *cobra.Command) {
	markRequired(cmd, "cluster-id")
}

// Reason is the justification for elevated access.
type Reason string

// AddRequired binds the required elevated-access reason flag.
func (f *Reason) AddRequired(cmd *cobra.Command, description string) {
	f.Add(cmd.Flags(), description)
	f.MarkRequired(cmd)
}

// Add binds the elevated-access reason flag.
func (f *Reason) Add(flags *pflag.FlagSet, description string) {
	flags.StringVar((*string)(f), "reason", string(*f), description)
}

// MarkRequired marks the elevated-access reason flag as required.
func (f *Reason) MarkRequired(cmd *cobra.Command) {
	markRequired(cmd, "reason")
}

// Destination is the target management cluster for recovery operations.
type Destination string

// Add binds the target management-cluster flag and its deprecated alias.
func (f *Destination) Add(cmd *cobra.Command, description string) {
	cmd.Flags().StringVar((*string)(f), "target-mc", string(*f), description)
	cmd.Flags().StringVar((*string)(f), "replacement-mc", string(*f), "Deprecated alias for --target-mc")
	if err := cmd.Flags().MarkDeprecated("replacement-mc", "use --target-mc instead"); err != nil {
		panic(fmt.Sprintf("deprecating flag %q: %v", "replacement-mc", err))
	}
}

// Validate reports a missing target management cluster.
func (f Destination) Validate() error {
	if strings.TrimSpace(string(f)) == "" {
		return fmt.Errorf("target management cluster is required; specify --target-mc")
	}
	return nil
}

// Source is an optional source management-cluster identifier.
type Source string

// Add binds the source management-cluster flag.
func (f *Source) Add(cmd *cobra.Command, description string) {
	cmd.Flags().StringVar((*string)(f), "source-mc", string(*f), description)
}

// AWSProfile is an optional AWS profile name.
type AWSProfile string

// Add binds the AWS profile flag.
func (f *AWSProfile) Add(cmd *cobra.Command, description string) {
	cmd.Flags().StringVarP((*string)(f), "profile", "p", string(*f), description)
}

func markRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(fmt.Sprintf("marking flag %q required: %v", name, err))
	}
}
