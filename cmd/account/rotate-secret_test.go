package account

import (
	"testing"

	awsSdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamTypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	mock_aws "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestRotateSecretOptions_Complete(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		flags           map[string]string
		expectedErr     bool
		expectedErrMsg  string
		expectedName    string
		expectedProfile string
	}{
		{
			name:           "missing_argument",
			args:           []string{},
			flags:          map[string]string{"reason": "OHSS-123"},
			expectedErr:    true,
			expectedErrMsg: "Account CR argument is required\nSee ' -h' for help and examples",
		},
		{
			name:            "valid_argument_and_flags",
			args:            []string{"my-account-cr"},
			flags:           map[string]string{"aws-profile": "custom", "reason": "PD-456"},
			expectedErr:     false,
			expectedName:    "my-account-cr",
			expectedProfile: "custom",
		},
		{
			name:            "default_profile_not_used",
			args:            []string{"another-cr"},
			flags:           map[string]string{"reason": "PD-789"},
			expectedErr:     false,
			expectedName:    "another-cr",
			expectedProfile: "",
		},
		{
			name:           "invalid_admin_username",
			args:           []string{"bad-cr"},
			flags:          map[string]string{"admin-username": "notosdManagedAdmin", "reason": "OHSS-000"},
			expectedErr:    true,
			expectedErrMsg: "admin-username must start with osdManagedAdmin\nSee ' -h' for help and examples",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := newRotateSecretOptions(genericclioptions.IOStreams{}, nil)
			cmd := &cobra.Command{}
			cmd.Flags().StringVar(&ops.profile, "aws-profile", "", "")
			cmd.Flags().BoolVar(&ops.updateCcsCreds, "ccs", false, "")
			cmd.Flags().StringVar(&ops.reason, "reason", "", "")
			cmd.Flags().StringVar(&ops.osdManagedAdminUsername, "admin-username", "", "")
			for k, v := range tt.flags {
				_ = cmd.Flags().Set(k, v)
			}
			err := ops.complete(cmd, tt.args)
			if tt.expectedErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedName, ops.accountCRName)
				assert.Equal(t, tt.expectedProfile, ops.profile)
				assert.NotNil(t, ops.awsAccountTimeout)
				assert.Equal(t, int32(900), *ops.awsAccountTimeout)
			}
		})
	}
}

func TestVerifyRotationPermissions(t *testing.T) {
	tests := []struct {
		name           string
		accountID      string
		username       string
		mockResponse   *iam.SimulatePrincipalPolicyOutput
		mockError      error
		expectedErr    bool
		expectedErrMsg string
	}{
		{
			name:      "all_permissions_allowed",
			accountID: "123456789012",
			username:  "osdManagedAdmin-test",
			mockResponse: &iam.SimulatePrincipalPolicyOutput{
				EvaluationResults: []iamTypes.EvaluationResult{
					{
						EvalActionName: awsSdk.String("iam:CreateAccessKey"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:CreateUser"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:DeleteAccessKey"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:DeleteUser"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:DeleteUserPolicy"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:GetUser"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:GetUserPolicy"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:ListAccessKeys"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:PutUserPolicy"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:TagUser"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
				},
			},
			mockError:   nil,
			expectedErr: false,
		},
		{
			name:      "some_permissions_denied",
			accountID: "123456789012",
			username:  "osdManagedAdmin-test",
			mockResponse: &iam.SimulatePrincipalPolicyOutput{
				EvaluationResults: []iamTypes.EvaluationResult{
					{
						EvalActionName: awsSdk.String("iam:CreateAccessKey"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeAllowed,
					},
					{
						EvalActionName: awsSdk.String("iam:CreateUser"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeExplicitDeny,
					},
					{
						EvalActionName: awsSdk.String("iam:DeleteAccessKey"),
						EvalDecision:   iamTypes.PolicyEvaluationDecisionTypeImplicitDeny,
					},
				},
			},
			mockError:      nil,
			expectedErr:    true,
			expectedErrMsg: "insufficient permissions for secret rotation. Denied actions: [iam:CreateUser iam:DeleteAccessKey]",
		},
		{
			name:           "simulate_api_error",
			accountID:      "123456789012",
			username:       "osdManagedAdmin-test",
			mockResponse:   nil,
			mockError:      assert.AnError,
			expectedErr:    true,
			expectedErrMsg: "failed to simulate principal policy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClient := mock_aws.NewMockClient(ctrl)

			expectedArn := "arn:aws:iam::" + tt.accountID + ":user/" + tt.username
			mockClient.EXPECT().
				SimulatePrincipalPolicy(gomock.Any()).
				DoAndReturn(func(input *iam.SimulatePrincipalPolicyInput) (*iam.SimulatePrincipalPolicyOutput, error) {
					// Verify the input is correct
					assert.Equal(t, expectedArn, *input.PolicySourceArn)
					assert.Len(t, input.ActionNames, 10)
					return tt.mockResponse, tt.mockError
				}).
				Times(1)

			err := verifyRotationPermissions(mockClient, tt.accountID, tt.username)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
