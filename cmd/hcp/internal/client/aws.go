package client

import (
	"fmt"

	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

// AWSAccountClientProvider creates AWS clients authenticated to an account.
type AWSAccountClientProvider interface {
	AccountClient(profile, region, accountID string) (awsprovider.Client, error)
}

type awsAccountClientProvider struct{}

// NewAWSAccountClientProvider creates the default account-scoped AWS client provider.
func NewAWSAccountClientProvider() AWSAccountClientProvider {
	return &awsAccountClientProvider{}
}

func (p *awsAccountClientProvider) AccountClient(profile, region, accountID string) (awsprovider.Client, error) {
	baseClient, err := awsprovider.NewAwsClient(profile, region, "")
	if err != nil {
		return nil, fmt.Errorf("creating AWS client: %w", err)
	}
	sessionName, err := osdCloud.GenerateRoleSessionName(baseClient)
	if err != nil {
		return nil, fmt.Errorf("generating AWS role session name: %w", err)
	}
	partition, err := awsprovider.GetAwsPartition(baseClient)
	if err != nil {
		return nil, fmt.Errorf("getting AWS partition: %w", err)
	}
	credentials, err := osdCloud.GenerateOrganizationAccountAccessCredentials(baseClient, accountID, sessionName, partition)
	if err != nil {
		return nil, fmt.Errorf("assuming account role: %w", err)
	}
	if incompleteCredentials(credentials) {
		return nil, fmt.Errorf("assuming account role returned incomplete credentials")
	}
	client, err := awsprovider.NewAwsClientWithInput(&awsprovider.ClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          region,
	})
	if err != nil {
		return nil, fmt.Errorf("creating account AWS client: %w", err)
	}
	return client, nil
}

func incompleteCredentials(credentials *ststypes.Credentials) bool {
	return credentials == nil || credentials.AccessKeyId == nil || credentials.SecretAccessKey == nil || credentials.SessionToken == nil
}
