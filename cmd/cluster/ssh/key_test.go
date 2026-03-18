package ssh

import (
	"strings"
	"testing"

	"github.com/openshift/osdctl/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	// Test namespaces
	namespace1 = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace1-abc",
		},
	}

	namespace2 = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace2-123",
		},
	}

	namespace3 = corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace3-xyz",
		},
	}
)

func Test_findClusterNamespace(t *testing.T) {
	type args struct {
		namespaces corev1.NamespaceList
		clusterID  string
	}
	tests := []struct {
		name      string
		args      args
		expected  corev1.Namespace
		expectErr bool
	}{
		{
			name: "Single valid namespace",
			args: args{
				clusterID: "abc",
				namespaces: corev1.NamespaceList{
					Items: []corev1.Namespace{namespace1, namespace2, namespace3},
				},
			},
			expected:  namespace1,
			expectErr: false,
		},
		{
			name: "No valid namespaces",
			args: args{
				clusterID: "invalidclusterid",
				namespaces: corev1.NamespaceList{
					Items: []corev1.Namespace{namespace1, namespace2, namespace3},
				},
			},
			expected:  corev1.Namespace{},
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := findClusterNamespace(test.args.namespaces, test.args.clusterID)
			// Check whether the error status is the one we expect
			if (err != nil) != test.expectErr {
				t.Errorf("mismatch between resulting error and expected error:\ngot:\n%v\n\nexpected:\n%v", err, test.expectErr)
				return
			}

			// Check the actual results of the test
			if result.Name != test.expected.Name {
				t.Errorf("mismatch between test result and expected value:\ngot:\n%#v\n\nexpected:\n%#v", result, test.expected)
			}
		})
	}
}

var (
	secret1 = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "secret1",
		},
	}

	secret2 = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "secret2",
		},
	}

	sshSecret = corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ssh",
		},
	}
)

// TestHiveOcmUrlValidation tests the early validation of --hive-ocm-url flag
func TestHiveOcmUrlValidation(t *testing.T) {
	tests := []struct {
		name        string
		hiveOcmUrl  string
		expectErr   bool
		errContains string
	}{
		{
			name:       "Valid hive-ocm-url (production)",
			hiveOcmUrl: "production",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (staging)",
			hiveOcmUrl: "staging",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (integration)",
			hiveOcmUrl: "integration",
			expectErr:  false,
		},
		{
			name:       "Valid hive-ocm-url (full URL)",
			hiveOcmUrl: "https://api.openshift.com",
			expectErr:  false,
		},
		{
			name:        "Invalid hive-ocm-url",
			hiveOcmUrl:  "invalid-environment",
			expectErr:   true,
			errContains: "invalid OCM_URL",
		},
		{
			name:        "Empty hive-ocm-url",
			hiveOcmUrl:  "",
			expectErr:   true,
			errContains: "empty OCM URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This simulates the validation that occurs in the RunE function
			var err error
			if tt.hiveOcmUrl != "" {
				_, err = utils.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
			} else {
				_, err = utils.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
			}

			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error containing '%s', but got nil", tt.errContains)
				} else if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("Expected error containing '%s', but got: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}
