package common

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
