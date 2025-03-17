package org

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unsafe"

	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/stretchr/testify/assert"
)

func TestSendRequest_Success(t *testing.T) {
	// Test constants
	var (
		tokenPath        = "/oauth2/token"
		testEndpoint     = "/test-endpoint"
		fakeClientID     = "fake-id"
		fakeClientSecret = "fake-secret"
	)

	// Test token response
	tokenResponse := `{
	"access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0LXVzZXIiLCJleHAiOjI1MjQ2MDgwMDB9.signature-placeholder", 
	"token_type": "Bearer", 
	"expires_in": 3600
	}`

	// Test API response
	apiResponse := `{"status": "OK"}`

	// HTTP handler function
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.URL.Path == tokenPath {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(tokenResponse))
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(apiResponse))
	}

	// Create server and connection
	server := httptest.NewServer(http.HandlerFunc(handler))
	defer server.Close()

	conn, err := sdk.NewConnectionBuilder().
		URL(server.URL).
		TokenURL(server.URL+tokenPath).
		Insecure(true).
		Client(fakeClientID, fakeClientSecret).
		Build()
	if err != nil {
		t.Fatalf("Failed to build connection: %v", err)
	}
	defer conn.Close()

	// Test the function
	req := conn.Get().Path(testEndpoint)
	_, err = sendRequest(req)

	if err != nil {
		t.Errorf("sendRequest returned an error: %v", err)
	}
}

func TestCreateGetSubscriptionsRequest(t *testing.T) {
	// Define test inputs
	fakeOrgID := "o-12345"
	fakeStatus := "Active"
	fakeManagedOnly := true
	fakePage := 1
	fakeSize := 20

	// Create a fake OCM client
	ocmClient := &sdk.Connection{}

	// Call the function under test
	request := createGetSubscriptionsRequest(ocmClient, fakeOrgID, fakeStatus, fakeManagedOnly, fakePage, fakeSize)

	// Assertions
	assert.NotNil(t, request, "Request should not be nil")
	assert.Equal(t, fakePage, getFieldValue(request, "page"), "Page number should be set correctly")
	assert.Equal(t, fakeSize, getFieldValue(request, "size"), "Size should be set correctly")

	// Verify search query format
	expectedQuery := fmt.Sprintf(`organization_id='%s' and status='%s' and managed=%v`, fakeOrgID, fakeStatus, fakeManagedOnly)
	assert.Equal(t, expectedQuery, getFieldValue(request, "search"), "Search query should match expected format")
}

func getFieldValue(v interface{}, fieldName string) interface{} {
	field := reflect.ValueOf(v).Elem().FieldByName(fieldName)
	if !field.IsValid() {
		return fmt.Sprintf("Field '%s' not found", fieldName)
	}
	if !field.CanInterface() {
		field = reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem()
	}
	if field.Kind() == reflect.Ptr && !field.IsNil() {
		return field.Elem().Interface()
	}
	return field.Interface()
}

func TestCheckOrgId(t *testing.T) {
	tests := []struct {
		Name          string
		Args          []string
		ErrorExpected bool
	}{
		{
			Name:          "Org Id provided",
			Args:          []string{"testOrgId"},
			ErrorExpected: false,
		},
		{
			Name:          "No Org Id provided",
			Args:          []string{},
			ErrorExpected: true,
		},
		{
			Name:          "Multiple Org id provided",
			Args:          []string{"testOrgId1", "testOrgId2"},
			ErrorExpected: true,
		},
	}

	for _, test := range tests {
		err := checkOrgId(test.Args)
		if test.ErrorExpected {
			if err == nil {
				t.Fatalf("Test '%s' failed. Expected error, but got none", test.Name)
			}
		} else {
			if err != nil {
				t.Fatalf("Test '%s' failed. Expected no error, but got '%v'", test.Name, err)
			}
		}
	}
}

func TestPrintJson(t *testing.T) {
	// Test Data
	testData := map[string]string{"name": "test", "value": "123"}

	// Capture Output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Call the function
	PrintJson(testData)

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)
	output := buf.String()

	// Expected JSON Format
	expectedJson, _ := json.MarshalIndent(testData, "", "  ")
	assert.Contains(t, output, string(expectedJson))
}

var jsonOutput bool

func TestPrintOrgTable(t *testing.T) {
	jsonOutput = false // Ensure table output

	org := Organization{
		ID:         "123",
		Name:       "Example Org",
		ExternalID: "ext123",
		Created:    "2023-01-01T00:00:00Z",
		Updated:    "2023-01-02T00:00:00Z",
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = w

	printOrg(org)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err = io.Copy(&buf, r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	assert.Len(t, lines, 6) // Assuming 6 lines, no empty line at end in test

	// Use regex to check each line, accounting for variable whitespace
	assert.Regexp(t, regexp.MustCompile(`^ID:\s+123$`), lines[0])
	assert.Regexp(t, regexp.MustCompile(`^Name:\s+Example Org$`), lines[1])
	assert.Regexp(t, regexp.MustCompile(`^External ID:\s+ext123$`), lines[2])
	assert.Regexp(t, regexp.MustCompile(`^Created:\s+2023-01-01T00:00:00Z$`), lines[4])
	assert.Regexp(t, regexp.MustCompile(`^Updated:\s+2023-01-02T00:00:00Z$`), lines[5])
}

func Test_getSubscriptions(t *testing.T) {
	t.Run("TestGetSubscriptions", func(t *testing.T) {
		// Create a mock OCM client
		got, err := getSubscriptions("org2", "active", true, 5, 10)
		if (err != nil) != false {
			t.Errorf("getSubscriptions() error = %v, wantErr %v", err, nil)
			return
		}
		if got.Page() <= 0 {
			t.Errorf("got.Page() = %v, want %v", got.Page(), 5)
		}
		if got.Status() != http.StatusOK {
			t.Errorf("got.Status() = %v, want %v", got.Status(), http.StatusOK )
		}
	})
}
