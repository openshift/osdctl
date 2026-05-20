package rhobs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	rhobsclient "github.com/observatorium/api/client"
)

type mcpLogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Stream    map[string]string `json:"stream,omitempty"`
}

func (q *RhobsFetcher) QueryAlerts(ctx context.Context) (json.RawMessage, error) {
	client, err := q.getClient()
	if err != nil {
		return nil, err
	}

	response, err := client.GetAlertsWithResponse(ctx, "hcp", &rhobsclient.GetAlertsParams{})
	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	return response.Body, nil
}

func (q *RhobsFetcher) QueryRules(ctx context.Context, ruleType string) (json.RawMessage, error) {
	client, err := q.getClient()
	if err != nil {
		return nil, err
	}

	params := &rhobsclient.GetRulesParams{}
	if ruleType != "" {
		params.Type = &ruleType
	}

	response, err := client.GetRulesWithResponse(ctx, "hcp", params)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to RHOBS: %v", err)
	}
	if response.HTTPResponse.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RHOBS query failed with status code: %d - body: %s", response.HTTPResponse.StatusCode, string(response.Body))
	}

	return response.Body, nil
}
