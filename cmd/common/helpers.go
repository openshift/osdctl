package common

import (
	"context"
	"fmt"

	bplogin "github.com/openshift/backplane-cli/cmd/ocm-backplane/login"
	bpconfig "github.com/openshift/backplane-cli/pkg/cli/config"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// UpdateSecret updates a specified k8s secret with the provided data
func UpdateSecret(kubeClient client.Client, secretName string, secretNamespace string, secretBody map[string][]byte) error {

	// Ensure the secret exists
	secret := &corev1.Secret{}
	err := kubeClient.Get(context.TODO(), types.NamespacedName{Name: secretName, Namespace: secretNamespace}, secret)
	if err != nil {
		return err
	}

	// Update secret
	secret.Data = secretBody
	err = kubeClient.Update(context.TODO(), secret)
	if err != nil {
		return err
	}

	return nil
}

// If some elevationReasons are provided, then the config will be elevated with user backplane-cluster-admin
func GetKubeConfigAndClient(clusterID string, elevationReasons ...string) (client.Client, *rest.Config, *kubernetes.Clientset, error) {
	bp, err := bpconfig.GetBackplaneConfiguration()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load backplane-cli config: %v", err)
	}
	var kubeconfig *rest.Config
	if len(elevationReasons) == 0 {
		kubeconfig, err = bplogin.GetRestConfig(bp, clusterID)
	} else {
		kubeconfig, err = bplogin.GetRestConfigAsUser(bp, clusterID, "backplane-cluster-admin", elevationReasons...)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	// create the clientset
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return nil, nil, nil, err
	}
	kubeCli, err := client.New(kubeconfig, client.Options{})
	if err != nil {
		return nil, nil, nil, err
	}
	return kubeCli, kubeconfig, clientset, err
}
