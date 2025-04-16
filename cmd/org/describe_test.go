package org

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestDescribeOrg(t *testing.T) {

	testToken, _ := jwt.New(jwt.SigningMethodHS256).SignedString([]byte("test-secret"))
	clientID := "fake-id"
	clientSecret := "fake-secret"
	tokenPath := "/fake-path/token"

	tokenResponse := map[string]interface{}{
		"access_token": testToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	testOrg := Organization{
		ID:           "test-org-id",
		Name:         "Test Organization",
		ExternalID:   "123456",
		EBSAccoundID: "789012",
		Created:      "2024-01-01T00:00:00Z",
		Updated:      "2024-01-01T00:00:00Z",
	}

	t.Run("Success Test", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
			json.NewEncoder(w).Encode(testOrg)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(false).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		cmd := &cobra.Command{}
		err = describeOrg(cmd, "test-org-id", conn)
		assert.NoError(t, err, "describeOrg() should not return an error")
	})

	t.Run("Error Test - Invalid Response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
			// Return invalid JSON
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(false).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		cmd := &cobra.Command{}
		err = describeOrg(cmd, "test-org-id", conn)
		assert.Error(t, err, "describeOrg() should return an error for invalid response")
	})

}
