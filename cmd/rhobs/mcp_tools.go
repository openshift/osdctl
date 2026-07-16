package rhobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type mcpLogEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Message   string            `json:"message"`
	Stream    map[string]string `json:"stream,omitempty"`
}

var readOnlyAnnotations = &mcp.ToolAnnotations{
	ReadOnlyHint:    true,
	DestructiveHint: boolPtr(false),
}

func registerMcpTools(s *mcp.Server) {
	s.AddTool(&mcp.Tool{
		Name: "rhobs_metrics",
		Description: "Query RHOBS Prometheus/Thanos metrics for ROSA HCP infrastructure. " +
			"Covers HCP hosted clusters, Management Clusters (MC), and Service Clusters (SC). " +
			"Accepts any cluster ID or name; the correct RHOBS cell is resolved automatically. " +
			"Instant query by default; add start/end for range query.",
		Annotations: readOnlyAnnotations,
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cluster_id":     {"type": "string", "description": "Cluster ID or name (HCP, MC, or SC)"},
				"query":          {"type": "string", "description": "PromQL expression"},
				"filter_cluster": {"type": "boolean", "description": "Filter to target cluster only. Default: true"},
				"start":          {"type": "string", "description": "Range query start (RFC3339 or Unix timestamp). Omit for instant query."},
				"end":            {"type": "string", "description": "Range query end (RFC3339 or Unix timestamp)"},
				"step":           {"type": "string", "description": "Range query step (e.g., 15s, 1m). Default: 60s"}
			},
			"required": ["cluster_id", "query"]
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cell":        {"type": "string", "description": "RHOBS cell URL (regional Thanos/Loki endpoint)"},
				"cluster_id":  {"type": "string", "description": "Internal cluster ID"},
				"environment": {"type": "string", "description": "OCM environment (production, stage, integration)"},
				"results":     {"type": "array", "description": "Metric results with labels and values"},
				"count":       {"type": "integer", "description": "Number of results"}
			}
		}`),
	}, handleMetrics)

	s.AddTool(&mcp.Tool{
		Name: "rhobs_logs",
		Description: "Query RHOBS Loki logs for ROSA HCP infrastructure. " +
			"Covers HCP hosted clusters, Management Clusters (MC), and Service Clusters (SC). " +
			"Accepts any cluster ID; HCP IDs are automatically resolved to their parent MC. " +
			"The correct RHOBS cell is resolved automatically.",
		Annotations: readOnlyAnnotations,
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cluster_id":     {"type": "string", "description": "Cluster ID or name (HCP, MC, or SC). HCP IDs auto-resolve to their parent MC."},
				"namespace":      {"type": "string", "description": "Kubernetes namespace. Required unless query is set."},
				"query":          {"type": "string", "description": "Raw LogQL expression (overrides namespace)"},
				"contain_regex":  {"type": "string", "description": "Server-side regex filter (e.g., (?i)(error|timeout))"},
				"since":          {"type": "string", "description": "Duration string (e.g., 1h, 30m). Default: 5m"},
				"limit":          {"type": "number", "description": "Max log entries. Default: 500, max: 10000"}
			},
			"required": ["cluster_id"]
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cell":        {"type": "string", "description": "RHOBS cell URL (regional Thanos/Loki endpoint)"},
				"cluster_id":  {"type": "string", "description": "Internal cluster ID"},
				"environment": {"type": "string", "description": "OCM environment (production, stage, integration)"},
				"query":       {"type": "string", "description": "LogQL expression that was executed"},
				"entries":     {"type": "array", "description": "Log entries with timestamp, message, and stream labels"},
				"count":       {"type": "integer", "description": "Number of log entries returned"}
			}
		}`),
	}, handleLogs)

	s.AddTool(&mcp.Tool{
		Name: "rhobs_alerts",
		Description: "Query firing alerts from RHOBS Alertmanager for ROSA HCP infrastructure. " +
			"Covers HCP hosted clusters, Management Clusters (MC), and Service Clusters (SC). " +
			"Accepts any cluster ID or name; the correct RHOBS cell is resolved automatically.",
		Annotations: readOnlyAnnotations,
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cluster_id": {"type": "string", "description": "Cluster ID or name (HCP, MC, or SC)"}
			},
			"required": ["cluster_id"]
		}`),
		OutputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"cell":        {"type": "string", "description": "RHOBS cell URL (regional Thanos/Loki endpoint)"},
				"cluster_id":  {"type": "string", "description": "Internal cluster ID"},
				"environment": {"type": "string", "description": "OCM environment (production, stage, integration)"},
				"alerts":      {"type": "array", "description": "Firing alerts with labels and annotations"},
				"count":       {"type": "integer", "description": "Number of alerts returned"}
			}
		}`),
	}, handleAlerts)
}

func getArgs(req *mcp.CallToolRequest) map[string]interface{} {
	var args map[string]interface{}
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return map[string]interface{}{}
	}
	return args
}

func getStringArg(args map[string]interface{}, key, defaultValue string) string {
	if val, ok := args[key].(string); ok {
		return val
	}
	return defaultValue
}

func getIntArg(args map[string]interface{}, key string, defaultValue int) int {
	if val, ok := args[key].(float64); ok {
		return int(val)
	}
	return defaultValue
}

func getBoolArg(args map[string]interface{}, key string, defaultValue bool) bool {
	if val, ok := args[key].(bool); ok {
		return val
	}
	return defaultValue
}

func handleMetrics(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	clusterId := getStringArg(args, "cluster_id", "")
	query := getStringArg(args, "query", "")
	filterCluster := getBoolArg(args, "filter_cluster", true)
	start := getStringArg(args, "start", "")
	end := getStringArg(args, "end", "")
	step := getStringArg(args, "step", "60s")

	if clusterId == "" || query == "" {
		return mcpError("cluster_id and query are required")
	}
	if (start == "") != (end == "") {
		return mcpError("start and end must be provided together for range queries")
	}

	fetcher, err := getCachedFetcher(ctx, clusterId, RhobsFetchForMetrics)
	if err != nil {
		return mcpError("Failed to initialize RHOBS fetcher: %v", err)
	}

	cellInfo := map[string]interface{}{
		"cell":        fetcher.RhobsCell,
		"cluster_id":  fetcher.clusterId,
		"environment": fetcher.ocmEnvName,
	}

	if start != "" && end != "" {
		results, err := fetcher.queryRangeMetrics(ctx, query, newRawMetricsTimeRange(start, end, step))
		if err != nil {
			return mcpError("Range metrics query failed: %v", err)
		}
		results = filterMetricsResults(fetcher, results, filterCluster)
		cellInfo["results"] = results
		cellInfo["count"] = len(*results)
		return mcpResultJSON(cellInfo)
	}

	results, err := fetcher.queryInstantMetrics(ctx, query, time.Time{})
	if err != nil {
		return mcpError("Metrics query failed: %v", err)
	}
	results = filterMetricsResults(fetcher, results, filterCluster)

	cellInfo["results"] = results
	cellInfo["count"] = len(*results)
	return mcpResultJSON(cellInfo)
}

func handleLogs(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	clusterId := getStringArg(args, "cluster_id", "")
	namespace := getStringArg(args, "namespace", "")
	rawQuery := getStringArg(args, "query", "")
	containRegex := getStringArg(args, "contain_regex", "")
	sinceStr := getStringArg(args, "since", "5m")
	limit := getIntArg(args, "limit", 500)

	if clusterId == "" {
		return mcpError("cluster_id is required")
	}
	if namespace == "" && rawQuery == "" {
		return mcpError("either namespace or query is required")
	}
	if limit <= 0 {
		return mcpError("limit must be greater than 0")
	}
	if limit > 10000 {
		limit = 10000
	}

	duration, err := time.ParseDuration(sinceStr)
	if err != nil {
		return mcpError("Invalid 'since' duration '%s': %v", sinceStr, err)
	}
	if duration <= 0 {
		return mcpError("'since' must be greater than 0")
	}

	fetcher, err := getCachedFetcher(ctx, clusterId, RhobsFetchForLogs)
	if err != nil {
		return mcpError("Failed to initialize RHOBS fetcher: %v", err)
	}

	var lokiExpr string
	if rawQuery != "" {
		lokiExpr = rawQuery
	} else {
		lokiExpr = fmt.Sprintf(`{k8s_namespace_name="%s"}`, namespace)
		if containRegex != "" {
			lokiExpr += fmt.Sprintf(` |~ "%s"`, containRegex)
		}
		lokiExpr += fmt.Sprintf(` | openshift_cluster_id = "%s"`, fetcher.logsClusterExtId())
	}

	now := time.Now()
	startTime := now.Add(-duration)

	entries := []mcpLogEntry{}
	err = fetcher.queryLogs(ctx, lokiExpr, startTime, now, limit, false, func(result *logResult) {
		entry := mcpLogEntry{
			Timestamp: result.getTime(),
			Message:   result.getMessage(),
		}
		if result.Stream != nil {
			entry.Stream = *result.Stream
		}
		entries = append(entries, entry)
	})
	if err != nil {
		return mcpError("Log query failed: %v", err)
	}

	return mcpResultJSON(map[string]interface{}{
		"cell":        fetcher.RhobsCell,
		"cluster_id":  fetcher.clusterId,
		"environment": fetcher.ocmEnvName,
		"query":       lokiExpr,
		"entries":     entries,
		"count":       len(entries),
	})
}

func handleAlerts(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := getArgs(req)
	clusterId := getStringArg(args, "cluster_id", "")

	if clusterId == "" {
		return mcpError("cluster_id is required")
	}

	fetcher, err := getCachedFetcher(ctx, clusterId, RhobsFetchForMetrics)
	if err != nil {
		return mcpError("Failed to initialize RHOBS fetcher: %v", err)
	}

	alerts, err := fetcher.queryAlerts(ctx)
	if err != nil {
		return mcpError("Alerts query failed: %v", err)
	}

	return mcpResultJSON(map[string]interface{}{
		"cell":        fetcher.RhobsCell,
		"cluster_id":  fetcher.clusterId,
		"environment": fetcher.ocmEnvName,
		"alerts":      alerts,
		"count":       len(*alerts),
	})
}
