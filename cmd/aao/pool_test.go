package aao

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type MockKubeClient struct {
	client.Client
	mock.Mock
}

func TestRun(t *testing.T) {
	tests := []struct {
		name            string
		mockAccounts    []v1alpha1.Account
		expectedOutputs []string
		expectError     bool
	}{
		{
			name: "TestRun_Success_With_Accounts",
			mockAccounts: []v1alpha1.Account{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "account1",
						Namespace: "aws-account-operator",
					},
					Status: v1alpha1.AccountStatus{
						Claimed: false,
						State:   "Ready",
					},
					Spec: v1alpha1.AccountSpec{
						LegalEntity: v1alpha1.LegalEntity{
							ID:   "",
							Name: "Entity1",
						},
						AccountPool: "default",
						BYOC:        false,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "account2",
						Namespace: "aws-account-operator",
					},
					Status: v1alpha1.AccountStatus{
						Claimed: true,
						State:   "Ready",
					},
					Spec: v1alpha1.AccountSpec{
						LegalEntity: v1alpha1.LegalEntity{
							ID:   "ID123",
							Name: "Entity2",
						},
						AccountPool: "fm-accountpool",
						BYOC:        false,
					},
				},
			},
			expectedOutputs: []string{
				"Total Accounts: 1",
				"Entity2",
			},
			expectError: false,
		},
		{
			name: "TestRun_NoAvailableAccounts",
			mockAccounts: []v1alpha1.Account{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "account3", 
						Namespace: "aws-account-operator",
					},
					Status: v1alpha1.AccountStatus{
						Claimed: true,
						State:   "Ready",
					},
					Spec: v1alpha1.AccountSpec{
						LegalEntity: v1alpha1.LegalEntity{
							ID:   "ID123",
							Name: "Entity3",
						},
						AccountPool: "default",
						BYOC:        false,
					},
				},
			},
			expectedOutputs: []string{
				"Total Accounts: 0", 
				"fm-accountpool",        
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up the scheme and add the necessary API types to the scheme
			scheme := runtime.NewScheme()
			if err := v1alpha1.AddToScheme(scheme); err != nil {
				t.Fatalf("Failed to add v1alpha1 to scheme: %v", err)
			}

			// Create a slice of client.Object for all the mock accounts
			objects := []client.Object{}
			for _, account := range tt.mockAccounts {
				objects = append(objects, &account)
			}

			// Create the fake Kubernetes client
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			// Create a buffer to capture stdout and stderr
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			// Create the poolOptions object and inject the fake client and buffers
			o := &poolOptions{
				kubeCli: client,
				IOStreams: genericclioptions.IOStreams{
					Out:    stdout,
					ErrOut: stderr,
					In:     nil,
				},
			}

			// Run the function and check for errors
			err := o.run()

			// If error is expected, assert error
			if tt.expectError {
				assert.Error(t, err)
				return
			}

			// Otherwise, assert no error
			assert.NoError(t, err)

			// Capture the printed output
			output := stdout.String()

			// Check if the expected outputs are in the result
			for _, expected := range tt.expectedOutputs {
				assert.Contains(t, output, expected, "Expected output to contain %q", expected)
			}
		})
	}
}

func TestHandlePoolCounting(t *testing.T) {
	tests := []struct {
		name        string
		initialMap  map[string]legalEntityStats
		account     v1alpha1.Account
		expectedMap map[string]legalEntityStats
	}{
		{
			name:       "Add_new_entry_for_unclaimed_account",
			initialMap: make(map[string]legalEntityStats),
			account: v1alpha1.Account{
				Spec: v1alpha1.AccountSpec{
					LegalEntity: v1alpha1.LegalEntity{
						ID:   "1",
						Name: "Entity1",
					},
				},
				Status: v1alpha1.AccountStatus{
					Claimed: false,
					State:   "Ready",
				},
			},
			expectedMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:        "Entity1",
					id:          "1",
					unusedCount: 1,
				},
			},
		},
		{
			name: "Update_unused_count_for_existing_unclaimed_account",
			initialMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:        "Entity1",
					id:          "1",
					unusedCount: 1,
				},
			},
			account: v1alpha1.Account{
				Spec: v1alpha1.AccountSpec{
					LegalEntity: v1alpha1.LegalEntity{
						ID:   "1",
						Name: "Entity1",
					},
				},
				Status: v1alpha1.AccountStatus{
					Claimed: false,
					State:   "Ready",
				},
			},
			expectedMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:        "Entity1",
					id:          "1",
					unusedCount: 2,
				},
			},
		},
		{
			name:       "Add_new_entry_for_claimed_account",
			initialMap: make(map[string]legalEntityStats),
			account: v1alpha1.Account{
				Spec: v1alpha1.AccountSpec{
					LegalEntity: v1alpha1.LegalEntity{
						ID:   "1",
						Name: "Entity1",
					},
				},
				Status: v1alpha1.AccountStatus{
					Claimed: true,
					State:   "Ready",
				},
			},
			expectedMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:         "Entity1",
					id:           "1",
					claimedCount: 1,
				},
			},
		},
		{
			name: "Update_claimed_count_for_existing_claimed_account",
			initialMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:         "Entity1",
					id:           "1",
					claimedCount: 1,
				},
			},
			account: v1alpha1.Account{
				Spec: v1alpha1.AccountSpec{
					LegalEntity: v1alpha1.LegalEntity{
						ID:   "1",
						Name: "Entity1",
					},
				},
				Status: v1alpha1.AccountStatus{
					Claimed: true,
					State:   "Ready",
				},
			},
			expectedMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:         "Entity1",
					id:           "1",
					claimedCount: 2,
				},
			},
		},
		{
			name: "Handle_mixed_accounts_with_same_LegalEntity_ID",
			initialMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:         "Entity1",
					id:           "1",
					claimedCount: 1,
				},
			},
			account: v1alpha1.Account{
				Spec: v1alpha1.AccountSpec{
					LegalEntity: v1alpha1.LegalEntity{
						ID:   "1",
						Name: "Entity1",
					},
				},
				Status: v1alpha1.AccountStatus{
					Claimed: false,
					State:   "Ready",
				},
			},
			expectedMap: map[string]legalEntityStats{
				"1 Entity1": {
					name:         "Entity1",
					id:           "1",
					claimedCount: 1,
					unusedCount:  1,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			handlePoolCounting(tt.initialMap, tt.account)

			assert.Equal(t, tt.expectedMap, tt.initialMap)
		})
	}
}
