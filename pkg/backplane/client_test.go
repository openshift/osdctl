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

	createJobResponse    *http.Response
	createJobError       error
	getRunResponse       *http.Response
	getRunError          error
	getLogsResponse      *http.Response
	getLogsError         error
	createReportResponse *http.Response
	createReportError    error
	getReportResponse    *http.Response
	getReportError       error
	listReportsResponse  *http.Response
	listReportsError     error
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

func (m *mockBackplaneClient) CreateReport(
	ctx context.Context,
	clusterId string,
	body backplaneapi.CreateReportJSONRequestBody,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.createReportResponse, m.createReportError
}

func (m *mockBackplaneClient) GetReportById(
	ctx context.Context,
	clusterId string,
	reportId string,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.getReportResponse, m.getReportError
}

func (m *mockBackplaneClient) GetReportsByCluster(
	ctx context.Context,
	clusterId string,
	params *backplaneapi.GetReportsByClusterParams,
	reqEditors ...backplaneapi.RequestEditorFn,
) (*http.Response, error) {
	return m.listReportsResponse, m.listReportsError
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
			client := newMockClient(mockClient, tt.clusterID)

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

			client := newMockClient(mockClient, "test-cluster")
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

			client := newMockClient(mockClient, "test-cluster")
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

func newMockClient(backplaneClient backplaneapi.ClientInterface, clusterID string) *Client {
	return &Client{
		backplaneClient: backplaneClient,
		clusterID:       clusterID,
	}
}

func TestCreateReport(t *testing.T) {
	tests := []struct {
		name          string
		summary       string
		data          string
		mockResponse  *http.Response
		mockError     error
		expectedError bool
		errorContains string
	}{
		{
			name:         "Success - 200 status",
			summary:      "Test Report",
			data:         "dGVzdCBkYXRh", // base64 encoded "test data"
			mockResponse: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"report_id":"report-123","summary":"Test Report","data":"dGVzdCBkYXRh","created_at":"2024-01-01T00:00:00Z"}`))},
		},
		{
			name:         "Success - 201 status",
			summary:      "Test Report",
			data:         "dGVzdCBkYXRh",
			mockResponse: &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"report_id":"report-456","summary":"Test Report","data":"dGVzdCBkYXRh","created_at":"2024-01-01T00:00:00Z"}`))},
		},
		{
			name:          "Client error",
			summary:       "Test Report",
			data:          "dGVzdCBkYXRh",
			mockError:     errors.New("network error"),
			expectedError: true,
			errorContains: "failed to create report",
		},
		{
			name:          "Non-2xx status code",
			summary:       "Test Report",
			data:          "dGVzdCBkYXRh",
			mockResponse:  &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader("bad request"))},
			expectedError: true,
			errorContains: "failed to create report, status: 400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{
				createReportResponse: tt.mockResponse,
				createReportError:    tt.mockError,
			}

			client := newMockClient(mockClient, "test-cluster")
			report, err := client.CreateReport(context.Background(), tt.summary, tt.data)

			if tt.expectedError {
				if err == nil {
					t.Errorf("CreateReport() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("CreateReport() error = %v, expected to contain %v", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("CreateReport() unexpected error = %v", err)
				}
				if report == nil {
					t.Errorf("CreateReport() report is nil")
				}
			}
		})
	}
}

func TestGetReport(t *testing.T) {
	tests := []struct {
		name          string
		reportID      string
		mockResponse  *http.Response
		mockError     error
		expectedError bool
		errorContains string
	}{
		{
			name:         "Success",
			reportID:     "report-123",
			mockResponse: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"report_id":"report-123","summary":"Test Report","data":"dGVzdCBkYXRh","created_at":"2024-01-01T00:00:00Z"}`))},
		},
		{
			name:          "Client error",
			reportID:      "report-456",
			mockError:     errors.New("network error"),
			expectedError: true,
			errorContains: "failed to get report",
		},
		{
			name:          "Invalid JSON response",
			reportID:      "report-789",
			mockResponse:  &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("invalid json"))},
			expectedError: true,
			errorContains: "failed to unmarshal report",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{
				getReportResponse: tt.mockResponse,
				getReportError:    tt.mockError,
			}

			client := newMockClient(mockClient, "test-cluster")
			report, err := client.GetReport(context.Background(), tt.reportID)

			if tt.expectedError {
				if err == nil {
					t.Errorf("GetReport() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("GetReport() error = %v, expected to contain %v", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("GetReport() unexpected error = %v", err)
				}
				if report == nil {
					t.Errorf("GetReport() report is nil")
				}
			}
		})
	}
}

func TestListReports(t *testing.T) {
	tests := []struct {
		name          string
		last          int
		mockResponse  *http.Response
		mockError     error
		expectedError bool
		errorContains string
	}{
		{
			name:         "Success with last parameter",
			last:         10,
			mockResponse: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"cluster_id":"test-cluster","reports":[{"report_id":"report-1","summary":"Report 1","created_at":"2024-01-01T00:00:00Z"}]}`))},
		},
		{
			name:         "Success without last parameter",
			last:         0,
			mockResponse: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"cluster_id":"test-cluster","reports":[]}`))},
		},
		{
			name:          "Client error",
			last:          10,
			mockError:     errors.New("network error"),
			expectedError: true,
			errorContains: "failed to list reports",
		},
		{
			name:          "Invalid JSON response",
			last:          10,
			mockResponse:  &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("invalid json"))},
			expectedError: true,
			errorContains: "failed to unmarshal reports",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &mockBackplaneClient{
				listReportsResponse: tt.mockResponse,
				listReportsError:    tt.mockError,
			}

			client := newMockClient(mockClient, "test-cluster")
			reports, err := client.ListReports(context.Background(), tt.last)

			if tt.expectedError {
				if err == nil {
					t.Errorf("ListReports() expected error but got none")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("ListReports() error = %v, expected to contain %v", err, tt.errorContains)
				}
			} else {
				if err != nil {
					t.Errorf("ListReports() unexpected error = %v", err)
				}
				if reports == nil {
					t.Errorf("ListReports() reports is nil")
				}
			}
		})
	}
}
