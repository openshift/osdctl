package access

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	clustersmgmtv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/k8s"
	osdctlutil "github.com/openshift/osdctl/pkg/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCleanupAccessOptions_dropPrivateLinkAccess(t *testing.T) {
	const (
		clusterid = "fake-cluster-uuid-12345"
	)

	tests := []struct {
		Name              string
		Pods              []metav1.ObjectMeta
		ExpectedPodsAfter []string
	}{
		{
			Name: "Single Jump Pod",
			Pods: []metav1.ObjectMeta{
				{
					Name:   "jump",
					Labels: map[string]string{jumpPodLabelKey: clusterid},
				},
			},
			ExpectedPodsAfter: []string{},
		},
		{
			Name:              "No pods",
			Pods:              []metav1.ObjectMeta{},
			ExpectedPodsAfter: []string{},
		},
		{
			Name: "Mixed use pods",
			Pods: []metav1.ObjectMeta{
				{
					Name:        "jump",
					Labels:      map[string]string{jumpPodLabelKey: clusterid},
					Annotations: map[string]string{"test-annotation": "test-annotation"},
				},
				{
					Name:   "provision",
					Labels: map[string]string{"a-provisioning-pod-label": "testing"},
				},
			},
			ExpectedPodsAfter: []string{"provision"},
		},
		{
			Name: "Multiple jump pods",
			Pods: []metav1.ObjectMeta{
				{
					Name:   "jump1",
					Labels: map[string]string{jumpPodLabelKey: clusterid},
				},
				{
					Name:   "jump2",
					Labels: map[string]string{jumpPodLabelKey: clusterid},
				},
			},
			ExpectedPodsAfter: []string{},
		},
	}

	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)

		// Generate test objects
		objs := []runtime.Object{}
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fmt.Sprintf("uhc-staging-%s", clusterid),
				Labels: map[string]string{"api.openshift.com/id": clusterid},
			},
		}
		objs = append(objs, &ns)

		for _, objMeta := range test.Pods {
			pod := corev1.Pod{
				ObjectMeta: objMeta,
			}
			pod.Namespace = ns.Name
			objs = append(objs, &pod)
		}

		// Setup Environment
		scheme := runtime.NewScheme()
		err := corev1.AddToScheme(scheme)
		if err != nil {
			t.Fatalf("Failed '%s': to add corev1 to scheme: %v", test.Name, err)
		}

		client := k8s.NewFakeClient(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...))
		streams := genericclioptions.IOStreams{In: strings.NewReader("y\n"), Out: os.Stdout, ErrOut: os.Stderr}
		cleanupAccess := newCleanupAccessOptions(client, streams)

		// Set the required "reason" flag for PrivateLink clusters
		cleanupAccess.reason = "testing-reason"

		cluster := generateClusterObjectForTesting("fake-cluster", clusterid, true, false)

		// Run test
		err = cleanupAccess.dropPrivateLinkAccess(&cluster, nil)

		// Verify results
		if err != nil {
			t.Fatalf("Failed '%s': unexpected error encountered: %v", test.Name, err)
		}

		// Verify only expected pods remain
		podsAfter := corev1.PodList{}
		err = cleanupAccess.kubeCli.List(context.TODO(), &podsAfter)
		if err != nil {
			t.Fatalf("Failed '%s': error while listing pods after testing: %v", test.Name, err)
		}

		if len(podsAfter.Items) != len(test.ExpectedPodsAfter) {
			t.Errorf("Failed '%s': unexpected number of pods remain after test: expected %d, got %d", test.Name, len(test.ExpectedPodsAfter), len(podsAfter.Items))
		}

		for _, pod := range podsAfter.Items {
			if !slices.Contains(test.ExpectedPodsAfter, pod.Name) {
				t.Errorf("Failed '%s': unexpected pod remains after test: %s", test.Name, pod.Name)
			}
		}
	}
}

func TestCleanupAccessOptions_dropPscAccess(t *testing.T) {
	const clusterid = "fake-psc-cluster-uuid"

	tests := []struct {
		Name              string
		Pods              []metav1.ObjectMeta
		ExpectedPodsAfter []string
	}{
		{
			Name: "PSC cluster - single jump pod",
			Pods: []metav1.ObjectMeta{
				{
					Name:   "jump",
					Labels: map[string]string{jumpPodLabelKey: clusterid},
				},
			},
			ExpectedPodsAfter: []string{},
		},
		{
			Name:              "PSC cluster - no pods",
			Pods:              []metav1.ObjectMeta{},
			ExpectedPodsAfter: []string{},
		},
		{
			Name: "PSC cluster - mixed pods",
			Pods: []metav1.ObjectMeta{
				{
					Name:   "jump",
					Labels: map[string]string{jumpPodLabelKey: clusterid},
				},
				{
					Name:   "provision",
					Labels: map[string]string{"a-provisioning-pod-label": "testing"},
				},
			},
			ExpectedPodsAfter: []string{"provision"},
		},
	}

	for _, test := range tests {
		fmt.Printf("Testing '%s'\n", test.Name)

		objs := []runtime.Object{}
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   fmt.Sprintf("uhc-staging-%s", clusterid),
				Labels: map[string]string{"api.openshift.com/id": clusterid},
			},
		}
		objs = append(objs, &ns)

		for _, objMeta := range test.Pods {
			pod := corev1.Pod{
				ObjectMeta: objMeta,
			}
			pod.Namespace = ns.Name
			objs = append(objs, &pod)
		}

		scheme := runtime.NewScheme()
		err := corev1.AddToScheme(scheme)
		if err != nil {
			t.Fatalf("Failed '%s': to add corev1 to scheme: %v", test.Name, err)
		}

		client := k8s.NewFakeClient(fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...))
		streams := genericclioptions.IOStreams{In: strings.NewReader("y\n"), Out: os.Stdout, ErrOut: os.Stderr}
		cleanupAccess := newCleanupAccessOptions(client, streams)
		cleanupAccess.reason = "testing-reason"

		cluster := generatePscClusterObjectForTesting("fake-psc-cluster", clusterid)

		err = cleanupAccess.dropPrivateLinkAccess(&cluster, nil)
		if err != nil {
			t.Fatalf("Failed '%s': unexpected error encountered: %v", test.Name, err)
		}

		podsAfter := corev1.PodList{}
		err = cleanupAccess.kubeCli.List(context.TODO(), &podsAfter)
		if err != nil {
			t.Fatalf("Failed '%s': error while listing pods after testing: %v", test.Name, err)
		}

		if len(podsAfter.Items) != len(test.ExpectedPodsAfter) {
			t.Errorf("Failed '%s': unexpected number of pods remain after test: expected %d, got %d", test.Name, len(test.ExpectedPodsAfter), len(podsAfter.Items))
		}

		for _, pod := range podsAfter.Items {
			if !slices.Contains(test.ExpectedPodsAfter, pod.Name) {
				t.Errorf("Failed '%s': unexpected pod remains after test: %s", test.Name, pod.Name)
			}
		}
	}
}

func TestCleanupHiveOcmUrlValidation(t *testing.T) {
	tests := []struct {
		name        string
		hiveOcmUrl  string
		expectError bool
	}{
		{name: "Valid (production)", hiveOcmUrl: "production", expectError: false},
		{name: "Valid (staging)", hiveOcmUrl: "staging", expectError: false},
		{name: "Valid (integration)", hiveOcmUrl: "integration", expectError: false},
		{name: "Invalid", hiveOcmUrl: "invalid-environment", expectError: true},
		{name: "Empty (flag omitted)", hiveOcmUrl: "", expectError: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.hiveOcmUrl != "" {
				_, err := osdctlutil.ValidateAndResolveOcmUrl(tt.hiveOcmUrl)
				if tt.expectError && err == nil {
					t.Errorf("expected error for hive-ocm-url %q, got nil", tt.hiveOcmUrl)
				}
				if !tt.expectError && err != nil {
					t.Errorf("unexpected error for hive-ocm-url %q: %v", tt.hiveOcmUrl, err)
				}
			}
		})
	}
}

func generatePscClusterObjectForTesting(name, id string) clustersmgmtv1.Cluster {
	cluster, err := clustersmgmtv1.NewCluster().
		Name(name).
		ID(id).
		GCP(clustersmgmtv1.NewGCP().PrivateServiceConnect(
			clustersmgmtv1.NewGcpPrivateServiceConnect().ServiceAttachmentSubnet("test-subnet"),
		)).
		API(clustersmgmtv1.NewClusterAPI().Listening(clustersmgmtv1.ListeningMethodExternal)).
		Build()

	if err != nil {
		panic(fmt.Sprintf("Failed to build PSC cluster: %v", err))
	}
	return *cluster
}
