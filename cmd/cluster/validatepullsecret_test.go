package cluster

import (
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func Test_getPullSecretEmail(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectedEmail string
		expectedError error
	}{
		{
			name:          "Missing dockerconfigjson",
			secret:        &corev1.Secret{Data: map[string][]byte{}},
			expectedEmail: "",
			expectedError: fmt.Errorf("secret does not contain expected key '.dockerconfigjson'"),
		},
		{
			name:          "Missing cloud.openshift.com auth",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{}}")}},
			expectedError: fmt.Errorf("secret does not contain entry for cloud.openshift.com"),
			expectedEmail: "",
		},
		{
			name:          "Missing email",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{}}}")}},
			expectedError: fmt.Errorf("empty email for auth: 'cloud.openshift.com' "),
			expectedEmail: "",
		},
		{
			name:          "Valid pull secret",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{\"email\":\"foo@bar.com\"}}}")}},
			expectedEmail: "foo@bar.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := getPullSecretEmail("abc123", tt.secret, "cloud.openshift.com", false)
			if email != tt.expectedEmail {
				t.Errorf("getPullSecretEmail() email = %v, expectedEmail %v", email, tt.expectedEmail)
			}
			if !reflect.DeepEqual(err, tt.expectedError) {
				t.Errorf("getPullSecretEmail() err = %v, expectedEmail %v", err, tt.expectedError)
			}
		})
	}
}
