package account

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestGenerateSecretOptions_Complete(t *testing.T) {
	tests := []struct {
		name             string
		args             []string
		flags            map[string]string
		expectedErr      bool
		expectedErrMsg   string
		expectedUsername string
		expectedAcctID   string
		expectedAcctName string
	}{
		{
			name:           "ccs_missing_account_name",
			args:           []string{},
			flags:          map[string]string{"ccs": "true"},
			expectedErr:    true,
			expectedErrMsg: "Account CR name argument is required\nSee ' -h' for help and examples",
		},
		{
			name:           "missing_iam_username_and_account_identifiers",
			args:           []string{},
			flags:          map[string]string{},
			expectedErr:    true,
			expectedErrMsg: "IAM User name argument is required\nSee ' -h' for help and examples",
		},
		{
			name:           "missing_account_name_and_id",
			args:           []string{"iam-user"},
			flags:          map[string]string{},
			expectedErr:    true,
			expectedErrMsg: "AWS account CR name and AWS account ID cannot be empty at the same time\nSee ' -h' for help and examples",
		},
		{
			name:           "both_account_name_and_id_provided",
			args:           []string{"iam-user"},
			flags:          map[string]string{"account-name": "acct-cr", "account-id": "123456789"},
			expectedErr:    true,
			expectedErrMsg: "AWS account CR name and AWS account ID cannot be set at the same time\nSee ' -h' for help and examples",
		},
		{
			name:             "valid_account_name",
			args:             []string{"iam-user"},
			flags:            map[string]string{"account-name": "acct-cr"},
			expectedErr:      false,
			expectedUsername: "iam-user",
			expectedAcctName: "acct-cr",
		},
		{
			name:             "valid_account_id",
			args:             []string{"iam-user"},
			flags:            map[string]string{"account-id": "123456789"},
			expectedErr:      false,
			expectedUsername: "iam-user",
			expectedAcctID:   "123456789",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := newGenerateSecretOptions(genericclioptions.IOStreams{}, nil)

			// Simulate cobra.Command context
			cmd := &cobra.Command{}
			cmd.Flags().StringVar(&ops.accountName, "account-name", "", "")
			cmd.Flags().StringVar(&ops.accountNamespace, "account-namespace", "", "")
			cmd.Flags().StringVar(&ops.accountID, "account-id", "", "")
			cmd.Flags().StringVar(&ops.profile, "aws-profile", "", "")
			cmd.Flags().StringVar(&ops.secretName, "secret-name", "", "")
			cmd.Flags().StringVar(&ops.secretNamespace, "secret-namespace", "", "")
			cmd.Flags().BoolVar(&ops.quiet, "quiet", false, "")
			cmd.Flags().BoolVar(&ops.ccs, "ccs", false, "")

			// Set flags
			for k, v := range tt.flags {
				_ = cmd.Flags().Set(k, v)
			}

			err := ops.complete(cmd, tt.args)

			if tt.expectedErr {
				assert.Error(t, err)
				assert.EqualError(t, err, tt.expectedErrMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedUsername, ops.iamUsername)
				assert.Equal(t, tt.expectedAcctID, ops.accountID)
				assert.Equal(t, tt.expectedAcctName, ops.accountName)
				assert.NotNil(t, ops.awsAccountTimeout)
				assert.Equal(t, int32(900), *ops.awsAccountTimeout)
			}
		})
	}
}
