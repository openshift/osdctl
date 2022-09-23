package k8s

import (
	"context"
	"encoding/base64"
	"fmt"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
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

// GetAccountClaimFromClusterID returns an account based on the cluster ID
func GetAccountClaimFromClusterID(
	ctx context.Context,
	cli client.Client,
	clusterID string,
) (*awsv1alpha1.AccountClaim, error) {
	var accountClaims awsv1alpha1.AccountClaimList
	labelSelector, err := labels.Parse(fmt.Sprintf("api.openshift.com/id=%s", clusterID))
	if err != nil {
		return nil, err
	}
	if err := cli.List(ctx, &accountClaims, &client.ListOptions{
		LabelSelector: labelSelector,
	}); err != nil {
		return nil, err
	}
	if len(accountClaims.Items) == 0 {
		return nil, nil
	}

	//There should only be one accountClaim
	return &accountClaims.Items[0], nil
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
		AccessKeyID:     string(accessKeyID),
		SecretAccessKey: string(secretAccessKey),
	}, nil
}

// Get AWS Account Claim CR
func GetAWSAccountClaim(
	ctx context.Context,
	cli client.Client,
	namespace,
	accountClaimName string,
) (*awsv1alpha1.AccountClaim, error) {
	var ac awsv1alpha1.AccountClaim
	if err := cli.Get(ctx, types.NamespacedName{
		Name:      accountClaimName,
		Namespace: namespace,
	}, &ac); err != nil {
		return nil, err
	}

	return &ac, nil
}

func NewAWSSecret(name, namespace, accessKeyID, secretAccessKey string) string {
	encodedAccessKeyID := base64.StdEncoding.EncodeToString([]byte(accessKeyID))
	encodedSecretAccessKey := base64.StdEncoding.EncodeToString([]byte(secretAccessKey))
	return fmt.Sprintf(`apiVersion: v1
data:
  aws_access_key_id: %s
  aws_secret_access_key: %s
kind: Secret
metadata:
  name: %s
  namespace: %s
type: Opaque
`, encodedAccessKeyID, encodedSecretAccessKey, name, namespace)
}
