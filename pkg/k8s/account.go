package k8s

import (
	"context"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// Get AWS Account CR
func GetAWSAccount(
	ctx context.Context,
	cli client.Client,
	namespace,
	accountCRName string,
) (*awsv1alpha1.Account, error) {
	var account awsv1alpha1.Account
	if err := cli.Get(ctx, types.NamespacedName{
		Name:      accountCRName,
		Namespace: namespace,
	}, &account); err != nil {
		return nil, err
	}

	return &account, nil
}

// Get the IAM Credentials created with AWS Account CR
func GetAWSAccountCredentials(
	ctx context.Context,
	cli client.Client,
	namespace,
	secretName string,
) (*awsprovider.AwsClientInput, error) {
	var secret v1.Secret
	if err := cli.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, &secret); err != nil {
		return nil, err
	}

	accessKeyID, ok := secret.Data["aws_access_key_id"]
	if !ok {
		return nil, fmt.Errorf("cannot find aws_access_key_id in secret %s", secretName)
	}
	secretAccessKey, ok := secret.Data["aws_secret_access_key"]
	if !ok {
		return nil, fmt.Errorf("cannot find aws_secret_access_key in secret %s", secretName)
	}

	return &awsprovider.AwsClientInput{
		AwsIDKey:     string(accessKeyID),
		AwsAccessKey: string(secretAccessKey),
	}, nil
}
