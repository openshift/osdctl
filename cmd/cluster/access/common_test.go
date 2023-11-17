package access

import (
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestIsAffirmative ensures the isAffirmative() function is operating as expected
func TestIsAffirmative(t *testing.T) {
	tests := []struct {
		Name           string
		Input          string
		ExpectedResult bool
	}{
		{
			Name:           "Y",
			Input:          "Y",
			ExpectedResult: true,
		},
		{
			Name:           "y",
			Input:          "y",
			ExpectedResult: true,
		},
		{
			Name:           "<blank>",
			Input:          "",
			ExpectedResult: false,
		},
		{
			Name:           "n",
			Input:          "n",
			ExpectedResult: false,
		},
		{
			Name:           "Garbage",
			Input:          "lj32423%#36",
			ExpectedResult: false,
		},
	}
	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)

		// Run test
		result := isAffirmative(test.Input)

		// Verify results
		if test.ExpectedResult != result {
			t.Errorf("Failed '%s': expected result to be '%t', got '%t'", test.Name, test.ExpectedResult, result)
		}
	}
}

// TestGetClusterNamespaces tests that getClusterNamespaces() retrieves the expected ns from hive
func TestGetClusterNamespaces(t *testing.T) {
	validClusterid := "fakecluster-123456"
	tests := []struct {
		Name              string
		Clusterid         string
		Namespaces        []metav1.ObjectMeta
		ExpectErr         bool
		ExpectedNamespace string
	}{
		{
			Name:      "Single valid namespace",
			Clusterid: validClusterid,
			Namespaces: []metav1.ObjectMeta{
				{
					Name:   "fake-namespace",
					Labels: map[string]string{hiveNSLabelKey: validClusterid},
				},
			},
			ExpectErr:         false,
			ExpectedNamespace: "fake-namespace",
		},
		{
			Name:       "No namespaces",
			Clusterid:  validClusterid,
			Namespaces: []metav1.ObjectMeta{},
			ExpectErr:  true,
		},
		{
			Name:      "No valid namespaces",
			Clusterid: validClusterid,
			Namespaces: []metav1.ObjectMeta{
				{
					Name:   "fake-namespace",
					Labels: map[string]string{hiveNSLabelKey: "invalid-cluster-id"},
				},
			},
			ExpectErr: true,
		},
		{
			Name:      "Multiple valid namespaces",
			Clusterid: validClusterid,
			Namespaces: []metav1.ObjectMeta{
				{
					Name:   "fake-namespace1",
					Labels: map[string]string{hiveNSLabelKey: validClusterid},
				},
				{
					Name:   "fake-namespace2",
					Labels: map[string]string{hiveNSLabelKey: validClusterid},
				},
			},
			ExpectErr: true,
		},
		{
			Name:      "Multiple namespaces, 1 valid",
			Clusterid: validClusterid,
			Namespaces: []metav1.ObjectMeta{
				{
					Name:   "valid-namespace",
					Labels: map[string]string{hiveNSLabelKey: validClusterid},
				},
				{
					Name:   "invalid-namespace1",
					Labels: map[string]string{hiveNSLabelKey: "invalid-cluster-id"},
				},
				{
					Name:   "invalid-namespace2",
					Labels: map[string]string{hiveNSLabelKey: "another-invalid-cluster-id"},
				},
			},
			ExpectErr:         false,
			ExpectedNamespace: "valid-namespace",
		},
	}
	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)
		// Setup environment
		objs := []runtime.Object{}
		for _, nsMeta := range test.Namespaces {
			ns := corev1.Namespace{
				ObjectMeta: nsMeta,
			}
			objs = append(objs, &ns)
		}

		scheme := runtime.NewScheme()
		err := corev1.AddToScheme(scheme)
		if err != nil {
			t.Fatalf("Failed '%s': could not add corev1 to scheme: %v", test.Name, err)
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

		// Run test
		ns, err := getClusterNamespace(client, test.Clusterid)

		// Verify results
		if test.ExpectErr {
			if err == nil {
				t.Errorf("Failed '%s': expected error, got none.", test.Name)
			}
		} else {
			if err != nil {
				t.Errorf("Failed '%s': unexpected error encountered when running test: %v", test.Name, err)
			}
		}

		if ns.Name != test.ExpectedNamespace {
			t.Errorf("Failed '%s': incorrect Namespace returned. Expected '%s', got '%s'", test.Name, test.ExpectedNamespace, ns.Name)
		}
	}
}
