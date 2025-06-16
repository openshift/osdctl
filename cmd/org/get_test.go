package org

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/spf13/cobra"
)

var (
	testOrgs = []Organization{
		{ID: "org1", Name: "Test Org 1", ExternalID: "ext1", EBSAccoundID: "ebs1"},
		{ID: "org2", Name: "Test Org 2", ExternalID: "ext2", EBSAccoundID: "ebs2"},
	}
)

func TestGetSearchType(t *testing.T) {
	tests := []struct {
		name               string
		searchUser         string
		searchEBSaccountID string
		expected           int
	}{
		{
			name:               "User search",
			searchUser:         "testuser",
			searchEBSaccountID: "",
			expected:           USER_SEARCH,
		},
		{
			name:               "EBS search",
			searchUser:         "",
			searchEBSaccountID: "123456",
			expected:           EBS_SEARCH,
		},
		{
			name:               "No search params",
			searchUser:         "",
			searchEBSaccountID: "",
			expected:           0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchUser = tt.searchUser
			searchEBSaccountID = tt.searchEBSaccountID
			result := getSearchType()
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetSearchQuery(t *testing.T) {
	tests := []struct {
		name               string
		searchUser         string
		searchEBSaccountID string
		isPartMatch        bool
		expected           string
	}{
		{
			name:               "User search without part match",
			searchUser:         "testuser",
			searchEBSaccountID: "",
			isPartMatch:        false,
			expected:           "search=username like 'testuser%'",
		},
		{
			name:               "User search with part match",
			searchUser:         "testuser",
			searchEBSaccountID: "",
			isPartMatch:        true,
			expected:           "search=username like '%testuser%'",
		},
		{
			name:               "EBS search",
			searchUser:         "",
			searchEBSaccountID: "123456",
			isPartMatch:        false,
			expected:           "search=ebs_account_id='123456'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchUser = tt.searchUser
			searchEBSaccountID = tt.searchEBSaccountID
			isPartMatch = tt.isPartMatch
			result := getSearchQuery()
			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestSearchOrgs(t *testing.T) {
	testToken, _ = jwt.New(jwt.SigningMethodHS256).SignedString([]byte("test-secret"))
	clientID = "fake-id"
	clientSecret = "fake-secret"   // #nosec G101
	tokenPath = "/fake-path/token" // #nosec G101
	tokenResponse := map[string]interface{}{
		"access_token": testToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	}

	t.Run("Success Test - User Search", func(t *testing.T) {
		searchUser = "testuser"
		searchEBSaccountID = ""

		apiResponse := AccountItems{
			AccountItems: []AccountItem{
				{Org: testOrgs[0]},
			},
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
		err = searchOrgs(cmd, conn)
		if err != nil {
			t.Fatalf("searchOrgs() returned an error: %v", err)
		}
	})

	t.Run("Success Test - EBS Search", func(t *testing.T) {
		searchUser = ""
		searchEBSaccountID = "123456"

		apiResponse := OrgItems{
			Orgs: []Organization{testOrgs[1]},
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
		err = searchOrgs(cmd, conn)
		if err != nil {
			t.Fatalf("searchOrgs() returned an error: %v", err)
		}
	})

	t.Run("Error Test - No Search Params", func(t *testing.T) {
		searchUser = ""
		searchEBSaccountID = ""

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if r.URL.Path == tokenPath {
				json.NewEncoder(w).Encode(tokenResponse)
				return
			}
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
		err = searchOrgs(cmd, conn)
		if err == nil {
			t.Error("Expected error for no search params, got nil")
		}
	})
}

func TestPrintOrgList(t *testing.T) {
	t.Run("Table Output", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		output = ""
		printOrgList(testOrgs)

		// Capture output and restore stdout
		w.Close()
		var buf bytes.Buffer
		_, err := buf.ReadFrom(r)
		if err != nil {
			t.Fatal("Failed to read from stdout pipe:", err)
		}
		os.Stdout = oldStdout

		output := buf.String()
		if !containsAll(output, "ID", "Name", "External ID", "EBS ID", "org1", "Test Org 1", "ext1", "ebs1") {
			t.Errorf("Expected output to contain organization data. Got:\n%s", output)
		}
	})

	t.Run("JSON Output", func(t *testing.T) {
		// Capture stdout
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		output = "json"
		printOrgList(testOrgs)

		// Capture output and restore stdout
		w.Close()
		var buf bytes.Buffer
		_, err := buf.ReadFrom(r)
		if err != nil {
			t.Fatal("Failed to read from stdout pipe:", err)
		}
		os.Stdout = oldStdout

		var items OrgItems
		err = json.Unmarshal(buf.Bytes(), &items)
		if err != nil {
			t.Fatalf("Failed to unmarshal JSON output: %v", err)
		}

		if !reflect.DeepEqual(items.Orgs, testOrgs) {
			t.Errorf("Expected organizations %+v, got %+v", testOrgs, items.Orgs)
		}
	})
}
