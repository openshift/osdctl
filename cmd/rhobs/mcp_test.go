package rhobs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func makeRequest(tool string, args map[string]interface{}) *mcp.CallToolRequest {
	argsJSON, _ := json.Marshal(args)
	return &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Name:      tool,
			Arguments: argsJSON,
		},
	}
}

func isToolError(result *mcp.CallToolResult) bool {
	return result != nil && result.IsError
}

func getResultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}

func parseResultJSON(t *testing.T, result *mcp.CallToolResult) map[string]interface{} {
	t.Helper()
	text := getResultText(result)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("result is not valid JSON: %v\ntext: %s", err, text)
	}
	return parsed
}

// --- getArgs tests ---

func TestGetArgs(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		req := makeRequest("test", map[string]interface{}{"key": "value"})
		args := getArgs(req)
		if args["key"] != "value" {
			t.Errorf("expected key=value, got %v", args["key"])
		}
	})

	t.Run("empty arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test",
				Arguments: json.RawMessage(`{}`),
			},
		}
		args := getArgs(req)
		if len(args) != 0 {
			t.Errorf("expected empty map, got %v", args)
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test",
				Arguments: json.RawMessage(`not json`),
			},
		}
		args := getArgs(req)
		if len(args) != 0 {
			t.Errorf("expected empty map for malformed JSON, got %v", args)
		}
	})

	t.Run("nil arguments", func(t *testing.T) {
		req := &mcp.CallToolRequest{
			Params: &mcp.CallToolParamsRaw{
				Name:      "test",
				Arguments: nil,
			},
		}
		args := getArgs(req)
		if len(args) != 0 {
			t.Errorf("expected empty map for nil arguments, got %v", args)
		}
	})
}

// --- Arg parser tests ---

func TestGetStringArg(t *testing.T) {
	args := map[string]interface{}{
		"name":  "test-cluster",
		"empty": "",
		"num":   42.0,
	}

	tests := []struct {
		key      string
		def      string
		expected string
	}{
		{"name", "default", "test-cluster"},
		{"empty", "default", ""},
		{"missing", "default", "default"},
		{"num", "default", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := getStringArg(args, tt.key, tt.def)
			if got != tt.expected {
				t.Errorf("getStringArg(%q) = %q, want %q", tt.key, got, tt.expected)
			}
		})
	}
}

func TestGetIntArg(t *testing.T) {
	args := map[string]interface{}{
		"count":    float64(42),
		"zero":     float64(0),
		"negative": float64(-5),
		"string":   "not a number",
		"bool":     true,
	}

	tests := []struct {
		key      string
		def      int
		expected int
	}{
		{"count", 10, 42},
		{"zero", 10, 0},
		{"negative", 10, -5},
		{"missing", 10, 10},
		{"string", 10, 10},
		{"bool", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := getIntArg(args, tt.key, tt.def)
			if got != tt.expected {
				t.Errorf("getIntArg(%q) = %d, want %d", tt.key, got, tt.expected)
			}
		})
	}
}

func TestGetBoolArg(t *testing.T) {
	args := map[string]interface{}{
		"yes":    true,
		"no":     false,
		"string": "true",
		"num":    1.0,
	}

	tests := []struct {
		key      string
		def      bool
		expected bool
	}{
		{"yes", false, true},
		{"no", true, false},
		{"missing", true, true},
		{"missing", false, false},
		{"string", false, false},
		{"num", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.key+"_def_"+boolStr(tt.def), func(t *testing.T) {
			got := getBoolArg(args, tt.key, tt.def)
			if got != tt.expected {
				t.Errorf("getBoolArg(%q, %v) = %v, want %v", tt.key, tt.def, got, tt.expected)
			}
		})
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// --- Result helper tests ---

func TestMcpResultJSON(t *testing.T) {
	t.Run("valid data", func(t *testing.T) {
		data := map[string]interface{}{"key": "value", "count": 42}
		result, err := mcpResultJSON(data)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		if isToolError(result) {
			t.Fatal("mcpResultJSON returned tool error")
		}
		parsed := parseResultJSON(t, result)
		if parsed["key"] != "value" {
			t.Errorf("expected key=value, got %v", parsed["key"])
		}
	})

	t.Run("nil data", func(t *testing.T) {
		result, err := mcpResultJSON(nil)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		text := getResultText(result)
		if text != "null" {
			t.Errorf("expected 'null', got %q", text)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		result, err := mcpResultJSON([]string{})
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		text := getResultText(result)
		if text != "[]" {
			t.Errorf("expected '[]', got %q", text)
		}
	})

	t.Run("structured content matches data", func(t *testing.T) {
		data := map[string]interface{}{"key": "value", "count": float64(42)}
		result, err := mcpResultJSON(data)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		if result.StructuredContent == nil {
			t.Fatal("StructuredContent is nil; expected the original data value")
		}
		got, ok := result.StructuredContent.(map[string]interface{})
		if !ok {
			t.Fatalf("StructuredContent type = %T, want map[string]interface{}", result.StructuredContent)
		}
		if got["key"] != "value" {
			t.Errorf("StructuredContent[key] = %v, want value", got["key"])
		}
		if got["count"] != float64(42) {
			t.Errorf("StructuredContent[count] = %v, want 42", got["count"])
		}
	})

	t.Run("structured content consistent with text content", func(t *testing.T) {
		type payload struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}
		data := payload{Name: "test", Value: 7}
		result, err := mcpResultJSON(data)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}

		// Text content must be valid JSON matching the struct.
		text := getResultText(result)
		var fromText payload
		if err := json.Unmarshal([]byte(text), &fromText); err != nil {
			t.Fatalf("text content is not valid JSON: %v", err)
		}
		if fromText.Name != data.Name || fromText.Value != data.Value {
			t.Errorf("text content = %+v, want %+v", fromText, data)
		}

		// StructuredContent must be the same original value.
		if result.StructuredContent != data {
			t.Errorf("StructuredContent = %+v, want %+v", result.StructuredContent, data)
		}
	})

	t.Run("structured content present alongside text in content slice", func(t *testing.T) {
		data := map[string]interface{}{"x": "y"}
		result, err := mcpResultJSON(data)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		if len(result.Content) == 0 {
			t.Fatal("Content slice is empty; text content must always be present")
		}
		if result.StructuredContent == nil {
			t.Fatal("StructuredContent must not be nil")
		}
	})

	t.Run("structured content serialises to valid JSON over wire", func(t *testing.T) {
		// Verify that StructuredContent round-trips cleanly through JSON serialisation,
		// which is what the MCP SDK does when sending the response over stdio/HTTP.
		data := map[string]interface{}{"metric": "up", "value": float64(1)}
		result, err := mcpResultJSON(data)
		if err != nil {
			t.Fatalf("mcpResultJSON returned error: %v", err)
		}
		b, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatalf("failed to marshal StructuredContent: %v", err)
		}
		var roundTripped map[string]interface{}
		if err := json.Unmarshal(b, &roundTripped); err != nil {
			t.Fatalf("StructuredContent round-trip produced invalid JSON: %v", err)
		}
		if roundTripped["metric"] != "up" {
			t.Errorf("round-tripped metric = %v, want up", roundTripped["metric"])
		}
	})
}

func TestMcpError(t *testing.T) {
	t.Run("formatted message", func(t *testing.T) {
		result, err := mcpError("test error: %s %d", "details", 42)
		if err != nil {
			t.Fatalf("mcpError returned Go error: %v", err)
		}
		if !isToolError(result) {
			t.Fatal("mcpError should return tool error")
		}
		text := getResultText(result)
		if text != "test error: details 42" {
			t.Errorf("error text = %q, want %q", text, "test error: details 42")
		}
	})

	t.Run("simple message", func(t *testing.T) {
		result, _ := mcpError("simple error")
		if !isToolError(result) {
			t.Fatal("expected tool error")
		}
		if getResultText(result) != "simple error" {
			t.Errorf("unexpected text: %s", getResultText(result))
		}
	})
}

func TestBoolPtr(t *testing.T) {
	truePtr := boolPtr(true)
	falsePtr := boolPtr(false)
	if *truePtr != true {
		t.Error("boolPtr(true) should be true")
	}
	if *falsePtr != false {
		t.Error("boolPtr(false) should be false")
	}
	// Verify they are distinct pointers
	if truePtr == falsePtr {
		t.Error("boolPtr should return distinct pointers")
	}
}

// --- handleMetrics validation tests ---

func TestHandleMetrics_MissingParams(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		expectedMsg string
	}{
		{"no args", map[string]interface{}{}, "cluster_id and query are required"},
		{"only cluster", map[string]interface{}{"cluster_id": "test"}, "cluster_id and query are required"},
		{"only query", map[string]interface{}{"query": "up"}, "cluster_id and query are required"},
		{"empty cluster", map[string]interface{}{"cluster_id": "", "query": "up"}, "cluster_id and query are required"},
		{"empty query", map[string]interface{}{"cluster_id": "test", "query": ""}, "cluster_id and query are required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_metrics", tt.args)
			result, err := handleMetrics(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error for missing params")
			}
			if got := getResultText(result); got != tt.expectedMsg {
				t.Errorf("error = %q, want %q", got, tt.expectedMsg)
			}
		})
	}
}

func TestHandleMetrics_PartialRangeArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]interface{}
	}{
		{"only start", map[string]interface{}{"cluster_id": "test", "query": "up", "start": "1234567890"}},
		{"only end", map[string]interface{}{"cluster_id": "test", "query": "up", "end": "1234567890"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_metrics", tt.args)
			result, err := handleMetrics(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error for partial range args")
			}
			text := getResultText(result)
			if text != "start and end must be provided together for range queries" {
				t.Errorf("unexpected error: %s", text)
			}
		})
	}
}

func TestHandleMetrics_BothStartEndPassesValidation(t *testing.T) {
	req := makeRequest("rhobs_metrics", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"query":      "up",
		"start":      "1234567890",
		"end":        "1234567900",
		"step":       "15s",
	})
	result, err := handleMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !isToolError(result) {
		t.Fatal("expected tool error (fetcher should fail)")
	}
	text := getResultText(result)
	if text == "start and end must be provided together for range queries" {
		t.Error("range validation should pass when both start and end are provided")
	}
}

func TestHandleMetrics_NeitherStartNorEndPassesValidation(t *testing.T) {
	req := makeRequest("rhobs_metrics", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"query":      "up",
	})
	result, err := handleMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !isToolError(result) {
		t.Fatal("expected tool error (fetcher should fail)")
	}
	text := getResultText(result)
	if text == "start and end must be provided together for range queries" {
		t.Error("should not get range validation error when neither start nor end provided")
	}
}

func TestHandleMetrics_DefaultFilterCluster(t *testing.T) {
	req := makeRequest("rhobs_metrics", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"query":      "up",
	})
	// Verify handler doesn't error on missing filter_cluster (defaults to true)
	result, err := handleMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Will fail at fetcher, but confirms filter_cluster default doesn't cause issues
	if !isToolError(result) {
		t.Error("expected fetcher error")
	}
}

func TestHandleMetrics_DefaultStep(t *testing.T) {
	req := makeRequest("rhobs_metrics", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"query":      "up",
		"start":      "1234567890",
		"end":        "1234567900",
	})
	result, err := handleMetrics(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Will fail at fetcher but confirms missing step doesn't panic (defaults to "60s")
	if !isToolError(result) {
		t.Error("expected fetcher error")
	}
}

// --- handleLogs validation tests ---

func TestHandleLogs_MissingParams(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		expectedMsg string
	}{
		{"no args", map[string]interface{}{}, "cluster_id is required"},
		{"only cluster", map[string]interface{}{"cluster_id": "test"}, "either namespace or query is required"},
		{"missing cluster", map[string]interface{}{"namespace": "default"}, "cluster_id is required"},
		{"empty cluster", map[string]interface{}{"cluster_id": "", "namespace": "default"}, "cluster_id is required"},
		{"no namespace or query", map[string]interface{}{"cluster_id": "test", "since": "5m"}, "either namespace or query is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_logs", tt.args)
			result, err := handleLogs(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error for missing params")
			}
			if got := getResultText(result); got != tt.expectedMsg {
				t.Errorf("error = %q, want %q", got, tt.expectedMsg)
			}
		})
	}
}

func TestHandleLogs_InvalidSinceDuration(t *testing.T) {
	tests := []struct {
		name  string
		since string
	}{
		{"bogus string", "bogus"},
		{"no unit", "30"},
		{"invalid unit", "5x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_logs", map[string]interface{}{
				"cluster_id": "will-fail-at-fetcher",
				"namespace":  "default",
				"since":      tt.since,
			})
			result, err := handleLogs(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error")
			}
		})
	}
}

func TestHandleLogs_LimitCapping(t *testing.T) {
	req := makeRequest("rhobs_logs", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"namespace":  "default",
		"limit":      float64(99999),
	})
	result, err := handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Will fail at fetcher, but proves handler didn't panic on large limit
	if !isToolError(result) {
		t.Error("expected tool error (invalid cluster)")
	}
}

func TestHandleLogs_NonPositiveLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit float64
	}{
		{"zero", 0},
		{"negative", -5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_logs", map[string]interface{}{
				"cluster_id": "test",
				"namespace":  "default",
				"limit":      tt.limit,
			})
			result, err := handleLogs(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error for non-positive limit")
			}
			if got := getResultText(result); got != "limit must be greater than 0" {
				t.Errorf("error = %q, want %q", got, "limit must be greater than 0")
			}
		})
	}
}

func TestHandleLogs_NonPositiveSince(t *testing.T) {
	req := makeRequest("rhobs_logs", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"namespace":  "default",
		"since":      "-5m",
	})
	result, err := handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	if !isToolError(result) {
		t.Error("expected tool error for negative since")
	}
	if got := getResultText(result); got != "'since' must be greater than 0" {
		t.Errorf("error = %q, want %q", got, "'since' must be greater than 0")
	}
}

func TestHandleLogs_DefaultSinceAndLimit(t *testing.T) {
	req := makeRequest("rhobs_logs", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"namespace":  "default",
	})
	result, err := handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Confirms default since="5m" and limit=500 don't cause issues
	if !isToolError(result) {
		t.Error("expected fetcher error")
	}
}

func TestHandleLogs_RawQueryPath(t *testing.T) {
	req := makeRequest("rhobs_logs", map[string]interface{}{
		"cluster_id": "will-fail-at-fetcher",
		"query":      `{k8s_namespace_name="custom"} |= "error"`,
	})
	result, err := handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Will fail at fetcher, but proves raw query path works (namespace not required)
	if !isToolError(result) {
		t.Error("expected fetcher error")
	}
}

func TestHandleLogs_NamespaceWithRegex(t *testing.T) {
	req := makeRequest("rhobs_logs", map[string]interface{}{
		"cluster_id":    "will-fail-at-fetcher",
		"namespace":     "openshift-monitoring",
		"contain_regex": "(?i)(error|timeout)",
	})
	result, err := handleLogs(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned Go error: %v", err)
	}
	// Proves namespace + contain_regex combination doesn't panic
	if !isToolError(result) {
		t.Error("expected fetcher error")
	}
}

// --- handleAlerts validation tests ---

func TestHandleAlerts_MissingCluster(t *testing.T) {
	tests := []struct {
		name        string
		args        map[string]interface{}
		expectedMsg string
	}{
		{"no args", map[string]interface{}{}, "cluster_id is required"},
		{"empty cluster", map[string]interface{}{"cluster_id": ""}, "cluster_id is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := makeRequest("rhobs_alerts", tt.args)
			result, err := handleAlerts(context.Background(), req)
			if err != nil {
				t.Fatalf("handler returned Go error: %v", err)
			}
			if !isToolError(result) {
				t.Error("expected tool error for missing cluster_id")
			}
			if got := getResultText(result); got != tt.expectedMsg {
				t.Errorf("error = %q, want %q", got, tt.expectedMsg)
			}
		})
	}
}

// --- Tool registration test ---

func TestRegisterMcpTools(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	registerMcpTools(s)

	// Use in-memory transport to verify tools are actually registered
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = s.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	expectedNames := map[string]bool{
		"rhobs_metrics": false,
		"rhobs_logs":    false,
		"rhobs_alerts":  false,
	}

	if len(result.Tools) != len(expectedNames) {
		t.Fatalf("expected %d tools, got %d", len(expectedNames), len(result.Tools))
	}

	for _, tool := range result.Tools {
		if _, ok := expectedNames[tool.Name]; !ok {
			t.Errorf("unexpected tool: %s", tool.Name)
		} else {
			expectedNames[tool.Name] = true
		}

		// Verify annotations
		if tool.Annotations == nil {
			t.Errorf("tool %s missing annotations", tool.Name)
			continue
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %s should be read-only", tool.Name)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("tool %s should not be destructive", tool.Name)
		}

		// Verify OutputSchema is set
		if tool.OutputSchema == nil {
			t.Errorf("tool %s missing OutputSchema", tool.Name)
		} else {
			schemaBytes, err := json.Marshal(tool.OutputSchema)
			if err != nil {
				t.Errorf("tool %s OutputSchema not marshalable: %v", tool.Name, err)
			} else {
				var schema map[string]interface{}
				if err := json.Unmarshal(schemaBytes, &schema); err != nil {
					t.Errorf("tool %s OutputSchema not valid JSON: %v", tool.Name, err)
				} else if schema["type"] != "object" {
					t.Errorf("tool %s OutputSchema type=%v, want object", tool.Name, schema["type"])
				}
			}
		}
	}

	for name, found := range expectedNames {
		if !found {
			t.Errorf("expected tool %q not registered", name)
		}
	}
}

func TestRegisterMcpTools_SchemaValidation(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	registerMcpTools(s)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = s.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}
	defer session.Close()

	result, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	for _, tool := range result.Tools {
		t.Run(tool.Name+"_schema", func(t *testing.T) {
			// Verify InputSchema is valid JSON
			schemaBytes, err := json.Marshal(tool.InputSchema)
			if err != nil {
				t.Fatalf("failed to marshal schema: %v", err)
			}
			var schema map[string]interface{}
			if err := json.Unmarshal(schemaBytes, &schema); err != nil {
				t.Fatalf("InputSchema is not valid JSON: %v", err)
			}

			// Verify it has the expected structure
			if schema["type"] != "object" {
				t.Errorf("expected type=object, got %v", schema["type"])
			}
			props, ok := schema["properties"].(map[string]interface{})
			if !ok {
				t.Fatal("missing properties")
			}

			// All tools should have cluster_id
			if _, ok := props["cluster_id"]; !ok {
				t.Error("missing cluster_id property")
			}

			// All tools should require cluster_id
			required, ok := schema["required"].([]interface{})
			if !ok {
				t.Fatal("missing required array")
			}
			hasClusterId := false
			for _, r := range required {
				if r == "cluster_id" {
					hasClusterId = true
				}
			}
			if !hasClusterId {
				t.Error("cluster_id should be required")
			}
		})
	}
}

// --- Roundtrip tool call test via in-memory transport ---

func TestToolCall_Roundtrip_ValidationErrors(t *testing.T) {
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	registerMcpTools(s)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = s.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}
	defer session.Close()

	tests := []struct {
		name        string
		tool        string
		args        map[string]interface{}
		expectedMsg string
	}{
		{
			"metrics missing query",
			"rhobs_metrics",
			map[string]interface{}{"cluster_id": "test"},
			"cluster_id and query are required",
		},
		{
			"metrics partial range",
			"rhobs_metrics",
			map[string]interface{}{"cluster_id": "test", "query": "up", "start": "123"},
			"start and end must be provided together for range queries",
		},
		{
			"logs missing namespace",
			"rhobs_logs",
			map[string]interface{}{"cluster_id": "test"},
			"either namespace or query is required",
		},
		{
			"alerts empty cluster",
			"rhobs_alerts",
			map[string]interface{}{"cluster_id": ""},
			"cluster_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      tt.tool,
				Arguments: tt.args,
			})
			if err != nil {
				t.Fatalf("CallTool returned protocol error: %v", err)
			}
			if !result.IsError {
				t.Error("expected tool error")
			}
			text := ""
			for _, c := range result.Content {
				if tc, ok := c.(*mcp.TextContent); ok {
					text = tc.Text
				}
			}
			if text != tt.expectedMsg {
				t.Errorf("error = %q, want %q", text, tt.expectedMsg)
			}
		})
	}
}

// --- Config subcommand test ---

func TestMcpConfigCommand(t *testing.T) {
	cmd := newCmdMcpConfig()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := cmd.RunE(cmd, []string{})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("config command returned error: %v", err)
	}

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(output), &config); err != nil {
		t.Fatalf("config output is not valid JSON: %v\noutput: %s", err, output)
	}

	servers, ok := config["mcpServers"].(map[string]interface{})
	if !ok {
		t.Fatal("missing mcpServers key")
	}

	rhobs, ok := servers["osdctl-rhobs"].(map[string]interface{})
	if !ok {
		t.Fatal("missing osdctl-rhobs server")
	}

	if _, ok := rhobs["command"].(string); !ok {
		t.Error("missing command field")
	}

	args, ok := rhobs["args"].([]interface{})
	if !ok {
		t.Fatal("missing args field")
	}
	if len(args) != 3 || args[0] != "rhobs" || args[1] != "mcp" || args[2] != "server" {
		t.Errorf("args = %v, want [rhobs mcp server]", args)
	}
}

// --- StructuredContent wire roundtrip test ---

// TestStructuredContent_WireRoundtrip creates a minimal in-process MCP server
// that returns a known mcpResultJSON payload and verifies that StructuredContent
// survives the full in-memory transport (same wire serialisation used by stdio).
// This guards against the SDK silently dropping the field on its way to the client.
func TestStructuredContent_WireRoundtrip(t *testing.T) {
	type response struct {
		Metric string  `json:"metric"`
		Value  float64 `json:"value"`
	}

	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	s.AddTool(&mcp.Tool{
		Name:        "echo_structured",
		Description: "Returns a fixed structured payload for testing.",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcpResultJSON(response{Metric: "up", Value: 1})
	})

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect failed: %v", err)
	}
	defer session.Close()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "echo_structured"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", getResultText(result))
	}

	// Text content must be present and parseable.
	text := getResultText(result)
	var fromText response
	if err := json.Unmarshal([]byte(text), &fromText); err != nil {
		t.Fatalf("text content not valid JSON: %v\ntext: %s", err, text)
	}
	if fromText.Metric != "up" || fromText.Value != 1 {
		t.Errorf("text content = %+v, want {up 1}", fromText)
	}

	// StructuredContent must survive the wire.
	if result.StructuredContent == nil {
		t.Fatal("StructuredContent is nil after wire roundtrip; MCP clients relying on structured output will not receive data")
	}
	b, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("cannot marshal StructuredContent received from wire: %v", err)
	}
	var fromStructured response
	if err := json.Unmarshal(b, &fromStructured); err != nil {
		t.Fatalf("StructuredContent from wire is not valid JSON: %v", err)
	}
	if fromStructured.Metric != "up" || fromStructured.Value != 1 {
		t.Errorf("StructuredContent from wire = %+v, want {up 1}", fromStructured)
	}
}

// --- Annotation tests ---

func TestToolAnnotations(t *testing.T) {
	if readOnlyAnnotations.ReadOnlyHint != true {
		t.Error("expected ReadOnlyHint=true")
	}
	if readOnlyAnnotations.DestructiveHint == nil || *readOnlyAnnotations.DestructiveHint {
		t.Error("expected DestructiveHint=false")
	}
}
