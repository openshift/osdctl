package cluster

import (
	corev1 "k8s.io/api/core/v1"
	"reflect"
	"testing"
)

func Test_getPullSecretEmail(t *testing.T) {
	tests := []struct {
		name          string
		secret        *corev1.Secret
		expectedEmail string
		expectedError error
		expectedDone  bool
	}{
		{
			name:         "Missing dockerconfigjson",
			secret:       &corev1.Secret{Data: map[string][]byte{}},
			expectedDone: true,
		},
		{
			name:         "Missing cloud.openshift.com auth",
			secret:       &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{}}")}},
			expectedDone: true,
		},
		{
			name:         "Missing email",
			secret:       &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{}}}")}},
			expectedDone: true,
		},
		{
			name:          "Valid pull secret",
			secret:        &corev1.Secret{Data: map[string][]byte{".dockerconfigjson": []byte("{\"auths\":{\"cloud.openshift.com\":{\"email\":\"foo@bar.com\"}}}")}},
			expectedEmail: "foo@bar.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			email, err, done := getPullSecretEmail("abc123", tt.secret, false)
			if email != tt.expectedEmail {
				t.Errorf("getPullSecretEmail() email = %v, expectedEmail %v", email, tt.expectedEmail)
			}
			if !reflect.DeepEqual(err, tt.expectedError) {
				t.Errorf("getPullSecretEmail() err = %v, expectedEmail %v", err, tt.expectedError)
			}
			if done != tt.expectedDone {
				t.Errorf("getPullSecretEmail() done = %v, expectedEmail %v", done, tt.expectedDone)
			}
		})
	}
}
