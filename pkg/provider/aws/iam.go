package aws

import (
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	"k8s.io/klog"
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
		if aerr, ok := err.(awserr.Error); ok {
			// the specified IAM User doesn't exist
			if aerr.Code() == iam.ErrCodeNoSuchEntityException {
				return false, nil
			}
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
				klog.Errorf("Failed to delete access key %s for user %s",
					*key.AccessKeyId, *username)
				return err
			}
		}
	}

	return nil
}
