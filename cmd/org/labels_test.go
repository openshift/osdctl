package org

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/spf13/cobra"
)

var (
	testLabels = []Label{
		{ID: "label1", Key: "key1", Value: "value1"},
		{ID: "label2", Key: "key2", Value: "value2"},
	}
)

func TestSearchLabelsByOrg(t *testing.T) {
	tokenResponse := map[string]interface{}{
		"access_token": testToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	t.Run("Success Test", func(t *testing.T) {
		apiResponse := LabelItems{
			Labels: testLabels,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
			json.NewEncoder(w).Encode(apiResponse)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(true).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		cmd := &cobra.Command{}
		err = searchLabelsByOrg(cmd, "test-org", conn)
		if err != nil {
			t.Fatalf("searchLabelsByOrg() returned an error: %v", err)
		}
	})

	t.Run("Error Test - Invalid Response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == tokenPath {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(true).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		cmd := &cobra.Command{}
		err = searchLabelsByOrg(cmd, "test-org", conn)
		if err != nil {
			t.Error("Expected error for invalid response, got nil")
		}
	})
}

func TestCreateGetLabelsRequest(t *testing.T) {
	tokenResponse := map[string]interface{}{
		"access_token": testToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	t.Run("Valid Org ID", func(t *testing.T) {
		apiResponse := LabelItems{
			Labels: testLabels,
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
			json.NewEncoder(w).Encode(apiResponse)
		}))
		defer server.Close()

		conn, err := sdk.NewConnectionBuilder().
			URL(server.URL).
			TokenURL(server.URL+tokenPath).
			Insecure(true).
			Client(clientID, clientSecret).
			Build()
		if err != nil {
			t.Fatalf("Failed to build connection: %v", err)
		}

		request := createGetLabelsRequest(conn, "test-org")
		if request == nil {
			t.Error("Expected non-nil request, got nil")
		}
	})
}

func TestPrintLabels(t *testing.T) {
	t.Run("Table Output", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		output = ""
		printLabels(testLabels)

		// Capture output and restore stdout
		w.Close()
		var buf bytes.Buffer
		_, err := buf.ReadFrom(r)
		if err != nil {
			t.Fatal("Failed to read from stdout pipe:", err)
		}
		os.Stdout = oldStdout

		output := buf.String()
		if !containsAll(output, "ID", "KEY", "VALUE", "label1", "key1", "value1") {
			t.Errorf("Expected output to contain label data. Got:\n%s", output)
		}
	})

	t.Run("JSON Output", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		output = "json"
		printLabels(testLabels)

		// Capture output and restore stdout
		w.Close()
		var buf bytes.Buffer
		_, err := buf.ReadFrom(r)
		if err != nil {
			t.Fatal("Failed to read from stdout pipe:", err)
		}
		os.Stdout = oldStdout

		var items LabelItems
		err = json.Unmarshal(buf.Bytes(), &items)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON output: %v", err)
		}

		if !reflect.DeepEqual(items.Labels, testLabels) {
			t.Errorf("Expected labels %+v, got %+v", testLabels, items.Labels)
		}
	})
}
