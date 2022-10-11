package servicequotas

import (
	"errors"
	"fmt"

	//"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/servicequotas"

	"github.com/spf13/cobra"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

// newCmdDescribe implements servicequotas describe
func newCmdDescribe() *cobra.Command {
	ops := newDescribeOptions()
	describeCmd := &cobra.Command{
		Use:               "describe",
		Short:             "Describe AWS service-quotas",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
		Aliases: []string{"describe-quotas", "describe-quota"},
	}

	describeCmd.Flags().StringVarP(&ops.queryServiceCode, "service-code", "", "ec2", "Query for ServiceCode")
	describeCmd.Flags().StringVarP(&ops.queryQuotaCode, "quota-code", "q", "L-1216C47A", "Query for QuotaCode")
	describeCmd.Flags().StringVarP(&ops.clusterID, "clusterID", "C", "", "Cluster ID")
	describeCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	describeCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return describeCmd
}

// describeOptions defines the struct for running list account command
type describeOptions struct {
	queryServiceCode string
	queryQuotaCode   string
	clusterID        string
	awsProfile       string

	verbose bool
}

func newDescribeOptions() *describeOptions {
	return &describeOptions{}
}

func (o *describeOptions) complete(cmd *cobra.Command) error {

	return nil
}

func (o *describeOptions) run() error {

	awsClient, err := osdCloud.GenerateAWSClientForCluster(o.awsProfile, o.clusterID)
	if err != nil {
		return err
	}

	var foundServiceQuotas []*servicequotas.ServiceQuota

	searchQuery := &servicequotas.ListServiceQuotasInput{
		ServiceCode: &o.queryServiceCode,
	}

	for {
		servicequotas, err := awsprovider.Client.ListServiceQuotas(awsClient, searchQuery)
		if err != nil {
			return err
		}

		for _, foundQuota := range servicequotas.Quotas {
			foundServiceQuotas = append(foundServiceQuotas, foundQuota)
		}

		// for pagination
		searchQuery.NextToken = servicequotas.NextToken
		if servicequotas.NextToken == nil {
			break
		}
	}

	found := false
	if o.queryQuotaCode == "" {
		fmt.Println(foundServiceQuotas)
	} else {
		for _, quota := range foundServiceQuotas {
			if *quota.QuotaCode == o.queryQuotaCode {
				fmt.Println(quota)
				found = true
			}
		}
	}
	if !found {
		return errors.New("Cannot find ServiceQuota (service:" + o.queryServiceCode + " quota:" + o.queryQuotaCode + ")")
	}

	return nil

}
