package backplane

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	backplaneapi "github.com/openshift/backplane-api/pkg/client"
)

// Mock client: embed the generated interface, override only what we use
type mockBackplaneClient struct {
	backplaneapi.ClientInterface

	createJobResponse *http.Response
	createJobError    error
	getRunResponse    *http.Response
	getRunError       error
	getLogsResponse   *http.Response
	getLogsError      error
}

func (m *mockBackplaneClient) CreateJob(
	ctx context.Context,
	clusterId string,
	body backplaneapi.CreateJobJSONRequestBody,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.createJobResponse, m.createJobError
}

func (m *mockBackplaneClient) GetRun(
	ctx context.Context,
	clusterId string,
	jobId string,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.getRunResponse, m.getRunError
}

func (m *mockBackplaneClient) GetJobLogs(
	ctx context.Context,
	clusterId string,
	jobId string,
	params *backplaneapi.GetJobLogsParams,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.getLogsResponse, m.getLogsError
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		clusterID string
	}{
		{
			name:      "Valid cluster ID",
			clusterID: "test-cluster-123",
		},
		{
			name:      "Empty cluster ID",
			clusterID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{}
			client := NewClient(mockClient, tt.clusterID)

			if client.clusterID != tt.clusterID {
				t.Errorf("NewClient() clusterID = %v, expected %v", client.clusterID, tt.clusterID)
			}
		})
	}
}

func TestGetJobLogs(t *testing.T) {
	tests := []struct {
		name          string
		jobID         string
		mockResponse  *http.Response
		mockError     error
		expectedLogs  string
		expectedError bool
		errorContains string
	}{
		{
			name:         "Success",
			jobID:        "job-123",
			mockResponse: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("test log output"))},
			expectedLogs: "test log output",
		},
		{
			name:          "Client error",
			jobID:         "job-456",
			mockError:     errors.New("network error"),
			expectedError: true,
			errorContains: "failed to get job logs",
		},
		{
			name:          "Non-200 status code",
			jobID:         "job-789",
			mockResponse:  &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found"))},
			expectedError: true,
			errorContains: "failed to retrieve job logs: 404",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{
				getLogsResponse: tt.mockResponse,
				getLogsError:    tt.mockError,
			}

			client := NewClient(mockClient, "test-cluster")
			logs, err := client.getJobLogs(tt.jobID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("getJobLogs() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("getJobLogs() error = %v, expected to contain %v", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("getJobLogs() unexpected error = %v", err)
				}
				if logs != tt.expectedLogs {
					t.Errorf("getJobLogs() logs = %v, expected %v", logs, tt.expectedLogs)
				}
			}
		})
	}
}

func TestRunManagedJobWithClient_CreateJobErrors(t *testing.T) {
	tests := []struct {
		name          string
		mockResponse  *http.Response
		mockError     error
		errorContains string
	}{
		{
			name:          "Network error",
			mockError:     errors.New("connection failed"),
			errorContains: "failed to create managed job",
		},
		{
			name:          "Non-200 status code",
			mockResponse:  &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("bad request"))},
			errorContains: "managed job creation failed with status: 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{
				createJobResponse: tt.mockResponse,
				createJobError:    tt.mockError,
			}

			client := NewClient(mockClient, "test-cluster")
			result, err := client.RunManagedJobWithClient("test-script", map[string]string{}, 60)

			if result != nil {
				t.Errorf("RunManagedJobWithClient() result = %v, expected nil", result)
			}
			if err == nil {
				t.Errorf("RunManagedJobWithClient() expected error but got none")
			}
			if !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("RunManagedJobWithClient() error = %v, expected to contain %v", err, tt.errorContains)
			}
		})
	}
}
