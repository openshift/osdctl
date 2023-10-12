package aws

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

func CreateIAMUserAndAttachPolicy(awsClient Client, username, policyArn *string) error {
	output, err := awsClient.CreateUser(&iam.CreateUserInput{UserName: username})
	if err != nil {
		return err
	}

	if _, err := awsClient.AttachUserPolicy(&iam.AttachUserPolicyInput{
		UserName:  output.User.UserName,
		PolicyArn: policyArn,
	}); err != nil {
		return err
	}

	return nil
}

func CheckIAMUserExists(awsClient Client, username *string) (bool, error) {
	if _, err := awsClient.GetUser(&iam.GetUserInput{UserName: username}); err != nil {
		var nse *types.NoSuchEntityException
		if errors.As(err, &nse) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func DeleteUserAccessKeys(awsClient Client, username *string) error {
	accessKeys, err := awsClient.ListAccessKeys(&iam.ListAccessKeysInput{UserName: username})
	if err != nil {
		return err
	}

	if accessKeys.AccessKeyMetadata != nil {
		for _, key := range accessKeys.AccessKeyMetadata {
			if _, err := awsClient.DeleteAccessKey(&iam.DeleteAccessKeyInput{
				UserName:    username,
				AccessKeyId: key.AccessKeyId,
			}); err != nil {
				return fmt.Errorf("failed to delete access key %s for user %s", *key.AccessKeyId, *username)
			}
		}
	}

	return nil
}

func GenerateRoleARN(accountId, roleName string) string {
	return fmt.Sprintf("arn:aws:iam::%s:role/%s", accountId, roleName)
}
