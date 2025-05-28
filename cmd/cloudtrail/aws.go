package cloudtrail

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// Whoami retrieves the AWS account ARN and account ID for the current caller
// using the provided STS client.
func Whoami(stsClient sts.Client) (accountArn string, accountId string, err error) {
	ctx := context.TODO()
	callerIdentityOutput, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", err
	}

	userArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", "", err
	}

	return userArn.String(), userArn.AccountID, nil
}
