package servicequotas

import (
	"errors"
	"fmt"

	//"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/servicequotas"

	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	k8spkg "github.com/openshift/osd-utils-cli/pkg/k8s"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// newCmdDescribe implements servicequotas describe
func newCmdDescribe(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *cobra.Command {
	ops := newDescribeOptions(streams, flags)
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

	ops.k8sclusterresourcefactory.AttachCobraCliFlags(describeCmd)

	describeCmd.Flags().StringVarP(&ops.queryServiceCode, "service-code", "", "ec2", "Query for ServiceCode")
	describeCmd.Flags().StringVarP(&ops.queryQuotaCode, "quota-code", "q", "L-1216C47A", "Query for QuotaCode")

	describeCmd.Flags().BoolVarP(&ops.allRegions, "all-regions", "", false, "Loop through all supported regions")

	describeCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")

	return describeCmd
}

// describeOptions defines the struct for running list account command
type describeOptions struct {
	k8sclusterresourcefactory k8spkg.ClusterResourceFactoryOptions

	queryServiceCode string
	queryQuotaCode   string

	verbose    bool
	allRegions bool

	genericclioptions.IOStreams
}

func newDescribeOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags) *describeOptions {
	return &describeOptions{
		k8sclusterresourcefactory: k8spkg.ClusterResourceFactoryOptions{
			Flags: flags,
		},
		IOStreams: streams,
	}
}

func (o *describeOptions) complete(cmd *cobra.Command) error {
	if valid, err := o.k8sclusterresourcefactory.ValidateIdentifiers(); !valid {
		if err != nil {
			return err
		}
	}

	if valid, err := o.k8sclusterresourcefactory.Awscloudfactory.ValidateIdentifiers(); !valid {
		if err != nil {
			return err
		}
	}

	if _, err := GetSupportedRegions(o.k8sclusterresourcefactory.Awscloudfactory.Region, o.allRegions); err != nil {
		return err
	}

	return nil
}

func (o *describeOptions) run() error {
	regions, error := GetSupportedRegions(o.k8sclusterresourcefactory.Awscloudfactory.Region, o.allRegions)
	if error != nil {
		return error
	}

	for _, region := range regions {
		if err := o.runOnceByRegion(region); err != nil {
			return err
		}
	}

	return nil
}

func (o *describeOptions) runOnceByRegion(region string) error {
	// override region in factory class
	o.k8sclusterresourcefactory.Awscloudfactory.Region = region

	awsClient, err := o.k8sclusterresourcefactory.GetCloudProvider(o.verbose)
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
