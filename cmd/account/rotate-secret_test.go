package account

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
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
