package aws

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/iam"
	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
)

type awsStatement struct {
	Effect    string                 `json:"Effect"`
	Action    []string               `json:"Action"`
	Resource  []string               `json:"Resource,omitempty"`
	Condition *awsv1alpha1.Condition `json:"Condition,omitempty"`
	Principal *awsv1alpha1.Principal `json:"Principal,omitempty"`
}

type policyDoc struct {
	Version   string         `json:"Version"`
	Statement []awsStatement `json:"Statement"`
}

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

func RefreshIAMPolicy(awsClient Client, federatedRole *awsv1alpha1.AWSFederatedRole, awsAccountID, uid string) error {
	roleName := federatedRole.Name + "-" + uid
	policyName := federatedRole.Spec.AWSCustomPolicy.Name + "-" + uid
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", awsAccountID, policyName)

	statements := make([]awsStatement, 0, len(federatedRole.Spec.AWSCustomPolicy.Statements))
	for _, sm := range federatedRole.Spec.AWSCustomPolicy.Statements {
		statement := awsStatement(sm)
		statements = append(statements, statement)
	}

	pd := policyDoc{
		Version:   "2012-10-17",
		Statement: statements,
	}

	payload, err := json.Marshal(&pd)
	if err != nil {
		return err
	}

	// detach the previous role policy
	// If noSuchEntity error, we can skip
	if _, err := awsClient.DetachRolePolicy(&iam.DetachRolePolicyInput{
		PolicyArn: &policyArn,
		RoleName:  &roleName,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != iam.ErrCodeNoSuchEntityException {
				return err
			}
		} else {
			return err
		}
	}

	// delete the previous policy
	// If noSuchEntity error, we can skip
	if _, err := awsClient.DeletePolicy(&iam.DeletePolicyInput{
		PolicyArn: &policyArn,
	}); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() != iam.ErrCodeNoSuchEntityException {
				return err
			}
		} else {
			return err
		}
	}

	// create new policy
	backoff := wait.Backoff{Duration: time.Second, Factor: 2, Steps: 3}
	if err := wait.ExponentialBackoff(backoff, func() (done bool, err error) {
		if _, err := awsClient.CreatePolicy(&iam.CreatePolicyInput{
			PolicyName:     &policyName,
			Description:    &federatedRole.Spec.AWSCustomPolicy.Description,
			PolicyDocument: aws.String(string(payload)),
		}); err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() == iam.ErrCodeEntityAlreadyExistsException {
					return false, nil
				}
			} else {
				return false, err
			}
		}
		return true, nil
	}); err != nil {
		return err
	}

	rolePolicies, err := awsClient.ListAttachedRolePolicies(&iam.ListAttachedRolePoliciesInput{
		RoleName: &roleName,
	})
	if err != nil {
		return err
	}

	policiesMap := make(map[string]struct{})
	for _, policy := range federatedRole.Spec.AWSManagedPolicies {
		policiesMap[policy] = struct{}{}
	}

	// two slices for polices/policyArns that need to attach and detach
	policyArnsToAttach := []string{policyArn}
	policyArnsToDetach := make([]string, 0)

	// check existing role policies
	for _, policy := range rolePolicies.AttachedPolicies {
		if _, ok := policiesMap[*policy.PolicyName]; ok {
			delete(policiesMap, *policy.PolicyName)
		} else {
			policyArnsToDetach = append(policyArnsToDetach, *policy.PolicyArn)
		}
	}

	for policy := range policiesMap {
		policyArnsToAttach = append(policyArnsToAttach, "arn:aws:iam::aws:policy/"+policy)
	}

	// Detach all removed polices from role
	for _, policyArn := range policyArnsToDetach {
		policyArn := policyArn // fix for implicit memory aliasing
		if _, err := awsClient.DetachRolePolicy(&iam.DetachRolePolicyInput{
			PolicyArn: &policyArn,
			RoleName:  &roleName,
		}); err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				if aerr.Code() != iam.ErrCodeNoSuchEntityException {
					return err
				}
			} else {
				return err
			}
		}
	}

	// Attach all needed polices to role
	for _, policyArn := range policyArnsToAttach {
		policyArn := policyArn // fix for implicit memory aliasing
		if _, err := awsClient.AttachRolePolicy(&iam.AttachRolePolicyInput{
			RoleName:  &roleName,
			PolicyArn: &policyArn,
		}); err != nil {
			return err
		}
	}

	return nil
}
