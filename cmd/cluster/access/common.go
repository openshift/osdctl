package access

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hiveNSLabelKey = "api.openshift.com/id"
)

// isAffirmative returns true if the provided input indicates user agreement ("y" or "Y")
func isAffirmative(input string) bool {
	return input == "y" || input == "Y"
}

// getClusterNamespace returns the hive namespace for a cluster given it's internal ID
func getClusterNamespace(client kclient.Client, clusterid string) (corev1.Namespace, error) {
	nsList := corev1.NamespaceList{}
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{hiveNSLabelKey: clusterid}}
	selector, err := metav1.LabelSelectorAsSelector(&labelSelector)
	if err != nil {
		return corev1.Namespace{}, err
	}

	err = client.List(context.TODO(), &nsList, &kclient.ListOptions{LabelSelector: selector})
	if err != nil {
		return corev1.Namespace{}, err
	}
	if len(nsList.Items) != 1 {
		return corev1.Namespace{}, fmt.Errorf("expected list operation to return exactly 1 namespace, got %d", len(nsList.Items))
	}

	return nsList.Items[0], nil
}
