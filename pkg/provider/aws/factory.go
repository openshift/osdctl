package aws

import (
	"github.com/aws/aws-sdk-go/service/sts"
	"github.com/openshift/osd-utils-cli/cmd/common"
	"github.com/spf13/cobra"
)

// FactoryOptions defines the struct for running list account command
type FactoryOptions struct {
	Region     string
	Profile    string
	ConfigFile string

	RoleName    string
	SessionName string

	ConsoleDuration int64

	Credentials *sts.Credentials

	CallerIdentity *sts.GetCallerIdentityOutput
}

// AttachCobraCliFlags adds cobra cli flags to cobra command
func (factory *FactoryOptions) AttachCobraCliFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&factory.Profile, "aws-profile", "p", "", "specify AWS profile")
	cmd.Flags().StringVarP(&factory.ConfigFile, "aws-config", "c", "", "specify AWS config file path")
	cmd.Flags().StringVarP(&factory.Region, "aws-region", "r", common.DefaultRegion, "specify AWS region")
	cmd.Flags().Int64VarP(&factory.ConsoleDuration, "duration", "d", 3600, "The duration of the console session. "+
		"Default value is 3600 seconds(1 hour)")
}

// ValidateIdentifiers checks for presence and validity of account identifiers
func (factory *FactoryOptions) ValidateIdentifiers() (bool, error) {
	return true, nil
}

// NewAwsClient checks for presence and validity of account identifiers
func (factory *FactoryOptions) NewAwsClient() (Client, error) {
	valid, err := factory.ValidateIdentifiers()
	if !valid {
		if err != nil {
			return nil, err
		}
	}

	return NewAwsClient(factory.Profile, factory.Region, factory.ConfigFile)
}
