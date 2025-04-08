package aao

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
)

type MockKubeClient struct {
	client.Client
	mock.Mock
}

func (m *MockKubeClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	args := m.Called(ctx, obj, opts)
	return args.Error(0)
}

func TestRun(t *testing.T) {
	tests := []struct {
		name           string
		mockReturnErr  error
		mockAccounts   []v1alpha1.Account
		expectedErr    error
		expectedCounts map[string]int // Expected claimed/unclaimed counts for each entity

	}{
		{
			name:          "TestRun_Success_With_Accounts",
			mockReturnErr: nil,
			mockAccounts: []v1alpha1.Account{
				{
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
			expectedErr: nil,
			expectedCounts: map[string]int{
				"Entity1": 1,
				"Entity2": 1,
			},
		},
		{
			name:           "TestRun_ListError",
			mockReturnErr:  errors.New("list error"),
			mockAccounts:   nil,
			expectedErr:    errors.New("list error"),
			expectedCounts: nil,
		},
		{
			name:          "TestRun_NoAvailableAccounts",
			mockReturnErr: nil,
			mockAccounts: []v1alpha1.Account{
				{
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
			expectedErr:    nil,
			expectedCounts: nil, // No "unclaimed" accounts, so counts should remain zero

		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKubeCli := &MockKubeClient{}
			// Mock the account list if error is expected
			mockKubeCli.On("List", mock.Anything, mock.Anything, mock.Anything).Return(tt.mockReturnErr)

			// Mock the account list if no error is expected
			if tt.mockReturnErr == nil {
				mockKubeCli.On("List", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
					arg := args.Get(1).(*v1alpha1.AccountList)
					arg.Items = tt.mockAccounts
				}).Return(nil)
			}

			o := &poolOptions{kubeCli: mockKubeCli}

			err := o.run()

			if err == nil && tt.expectedErr != nil {
				t.Errorf("expected error '%v', got nil", tt.expectedErr)
			}

			if err != nil && err.Error() != tt.expectedErr.Error() {
				t.Errorf("expected error '%v', got '%v'", tt.expectedErr, err)
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
