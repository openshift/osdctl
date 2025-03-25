package cluster

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func Test_getPullSecretAuthEmail(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectedEmail string
		expectedError error
	}{
		{
			name:          "Missing dockerconfigjson",
			secret:        &corev1.Secret{Data: map[string][]byte{}},
			expectedError: ErrSecretMissingDockerConfigJson,
		},
		{
			name:          "Missing cloud.openshift.com auth",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{}}")}},
			expectedError: &ErrorSecretAuthNotFound{auth: "cloud.openshift.com"},
		},
		{
			name:          "Missing email",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{}}}")}},
			expectedError: &ErrorAuthEmailNotFound{auth: "cloud.openshift.com"},
		},
		{
			name:          "Valid pull secret",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{\"email\":\"foo@bar.com\"}}}")}},
			expectedEmail: "foo@bar.com",
			expectedError: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err := getPullSecretAuthEmail(tt.secret, "cloud.openshift.com")
			if email != tt.expectedEmail {
				t.Errorf("getPullSecretEmail() email = %v, expectedEmail %v", email, tt.expectedEmail)
			}
			if !reflect.DeepEqual(err, tt.expectedError) {
				t.Errorf("getPullSecretEmail() err = %v, expectedEmail %v", err, tt.expectedError)
			}
			//if err != nil {
			//	fmt.Fprintf(os.Stderr, "Got error type:'%T' vs Expected:'%T\n", err, tt.expectedError)
			//}
		})
	}
}
