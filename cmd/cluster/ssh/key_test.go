package ssh

import (
	"testing"

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
