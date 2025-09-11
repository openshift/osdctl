package backplane

import (
	"context"
	"fmt"
	"io"
	"time"

	backplaneapi "github.com/openshift/backplane-api/pkg/client"
	"k8s.io/apimachinery/pkg/util/wait"
)

type Client struct {
	backplaneClient backplaneapi.ClientInterface
	clusterID       string
}

// NewClient creates a new backplane client
func NewClient(backplaneClient backplaneapi.ClientInterface, clusterID string) *Client {
	return &Client{
		backplaneClient: backplaneClient,
		clusterID:       clusterID,
	}
}

type ManagedJobResult struct {
	Output string
	JobID  string
}

// RunManagedJobWithClient executes a managedscript (with a specified timeout) on the cluster and returns the result
func (c *Client) RunManagedJobWithClient(canonicalName string, parameters map[string]string, timeoutSeconds int) (*ManagedJobResult, error) {
	createJob := backplaneapi.CreateJobJSONRequestBody{
		CanonicalName: &canonicalName,
		Parameters:    &parameters,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	fmt.Printf("\nCreating managed job for script: %s on cluster: %s\n", canonicalName, c.clusterID)
	resp, err := c.backplaneClient.CreateJob(ctx, c.clusterID, createJob)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("timeout deadline reached: was unable to create the job within the deadline")
		}
		return nil, fmt.Errorf("failed to create managed job: %w", err)
	}

	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("managed job creation failed with status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	job, err := backplaneapi.ParseCreateJobResponse(resp)
	if err != nil {
		return nil, fmt.Errorf("failed to parse job creation response: %w", err)
	}

	if job.JSON200 == nil || job.JSON200.JobId == nil {
		return nil, fmt.Errorf("no job ID returned from create job")
	}

	jobID := *job.JSON200.JobId
	fmt.Printf("Job %s created. Waiting for it to finish running. (Timeout in %v seconds)\n", jobID, timeoutSeconds)

	err = c.waitForJobCompletion(jobID)
	if err != nil {
		return nil, fmt.Errorf("managed job did not complete successfully: %w", err)
	}

	output, err := c.getJobLogs(jobID)
	if err != nil {
		return nil, fmt.Errorf("failed to get job logs: %w", err)
	}

	return &ManagedJobResult{
		Output: output,
		JobID:  jobID,
	}, nil
}

func (c *Client) waitForJobCompletion(jobID string) error {
	pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return wait.PollUntilContextTimeout(pollCtx, 5*time.Second, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		runResp, err := c.backplaneClient.GetRun(ctx, c.clusterID, jobID)
		if err != nil {
			return false, fmt.Errorf("failed to get job status: %w", err)
		}

		if runResp.StatusCode != 200 {
			bodyBytes, _ := io.ReadAll(runResp.Body)
			runResp.Body.Close()
			return false, fmt.Errorf("failed to get job status: %d, body: %s", runResp.StatusCode, string(bodyBytes))
		}

		run, err := backplaneapi.ParseGetRunResponse(runResp)
		if err != nil {
			return false, fmt.Errorf("failed to parse job status response: %w", err)
		}

		if run.JSON200 == nil || run.JSON200.JobStatus == nil || run.JSON200.JobStatus.Status == nil {
			return false, nil
		}

		status := *run.JSON200.JobStatus.Status
		fmt.Printf("Job status: %s\n", status)

		switch status {
		case backplaneapi.JobStatusStatusSucceeded:
			return true, nil
		case backplaneapi.JobStatusStatusFailed:
			return false, fmt.Errorf("job failed with status: %s", status)
		case backplaneapi.JobStatusStatusKilled:
			return false, fmt.Errorf("job was killed")
		default:
			return false, nil
		}
	})
}

func (c *Client) getJobLogs(jobID string) (string, error) {
	v2 := "v2"
	logsParams := &backplaneapi.GetJobLogsParams{
		Version: &v2,
	}
	logsResp, err := c.backplaneClient.GetJobLogs(context.Background(), c.clusterID, jobID, logsParams)
	if err != nil {
		return "", fmt.Errorf("failed to get job logs: %w", err)
	}

	if logsResp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(logsResp.Body)
		logsResp.Body.Close()
		return "", fmt.Errorf("failed to retrieve job logs: %d, body: %s", logsResp.StatusCode, string(bodyBytes))
	}

	logBytes, err := io.ReadAll(logsResp.Body)
	logsResp.Body.Close()
	if err != nil {
		return "", fmt.Errorf("failed to read job logs: %w", err)
	}

	return string(logBytes), nil
}
