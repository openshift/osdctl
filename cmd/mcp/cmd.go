package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openshift/osdctl/cmd/cluster"
	"github.com/openshift/osdctl/cmd/servicelog"
	"github.com/spf13/cobra"
)

// This is reusable Input type for commands that only need the cluster id and nothing else.
type ClusterIdInput struct {
	ClusterId string `json:"cluster_id" jsonschema:"ID of the cluster to retrieve information for"`
}

var ClusterIdInputSchema, _ = jsonschema.For[ClusterIdInput](&jsonschema.ForOptions{})

// This is just the most generic type of output. Used for e.g. the context command that already provides a JSON output
// option, but the data types that make it up can't be converted to a JSONSCHEMA because Jira types are
// self-referential.
type MCPStringOutput struct {
	Context string `json:"context"`
}

var MCPStringOutputSchema, _ = jsonschema.For[MCPStringOutput](&jsonschema.ForOptions{})

type MCPServiceLogInput struct {
	ClusterId string `json:"cluster_id" jsonschema:"ID of the cluster to retrieve information for"`
	Internal  bool   `json:"internal" jsonschema:"Include internal servicelogs"`
	All       bool   `json:"all" jsonschema:"List all servicelogs"`
}

var MCPServiceLogInputSchema, _ = jsonschema.For[MCPServiceLogInput](&jsonschema.ForOptions{})

type MCPServiceLogOutput struct {
	ServiceLogs servicelog.LogEntryResponseView `json:"service_logs"`
}

var MCPServiceLogOutputSchema, _ = jsonschema.For[MCPServiceLogOutput](&jsonschema.ForOptions{})

var MCPCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start osdctl in MCP server mode",
	Long:  "Start osdctl as model-context-protocol server for integration with AI assistants.",
	Args:  cobra.ExactArgs(0),
	RunE:  runMCP,
}

func init() {
	MCPCmd.Flags().Bool("http", false, "Use an HTTP server instead of stdio")
	MCPCmd.Flags().Int("port", 8080, "HTTP Server port to use when running in HTTP mode")
}

func runMCP(cmd *cobra.Command, argv []string) error {
	useHttp, _ := cmd.Flags().GetBool("http")
	httpPort, _ := cmd.Flags().GetInt("port")
	if useHttp {
		fmt.Println("HTTP mode selected")
	}
	server := mcp.NewServer(&mcp.Implementation{Name: "osdctl", Version: "v0.0.1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "context",
		Description:  "Retrieve cluster context for a given cluster id",
		InputSchema:  ClusterIdInputSchema,
		OutputSchema: MCPStringOutputSchema,
		Title:        "cluster context",
	}, GenerateContext)
	mcp.AddTool(server, &mcp.Tool{
		Name:         "service_logs",
		Description:  "Retrieve cluster service logs for a given cluster id",
		InputSchema:  MCPServiceLogInputSchema,
		OutputSchema: MCPServiceLogOutputSchema,
		Title:        "cluster service logs",
	}, ListServiceLogs)
	if useHttp {
		// Create the streamable HTTP handler.
		handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
			return server
		}, nil)
		server := &http.Server{
			Addr:              fmt.Sprintf("http://localhost:%d", httpPort),
			ReadHeaderTimeout: 3 * time.Second,
		}
		http.Handle("/", handler)
		if err := server.ListenAndServe(); err != nil {
			return err
		}
	}
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		return err
	}
	return nil
}

func GenerateContext(ctx context.Context, req *mcp.CallToolRequest, input ClusterIdInput) (*mcp.CallToolResult, MCPStringOutput, error) {
	context, _ := cluster.GenerateContextData(input.ClusterId)
	return nil, MCPStringOutput{
		context,
	}, nil
}

func ListServiceLogs(ctx context.Context, req *mcp.CallToolRequest, input MCPServiceLogInput) (*mcp.CallToolResult, MCPServiceLogOutput, error) {
	output := MCPServiceLogOutput{}
	serviceLogs, err := servicelog.FetchServiceLogs(input.ClusterId, input.All, input.Internal)
	if err != nil {
		return &mcp.CallToolResult{
			Meta:              mcp.Meta{},
			Content:           []mcp.Content{},
			StructuredContent: nil,
			IsError:           true,
		}, output, err
	}
	view := servicelog.ConvertOCMSlToLogEntryView(serviceLogs)
	output.ServiceLogs = view
	return &mcp.CallToolResult{
		Meta:              mcp.Meta{},
		Content:           []mcp.Content{},
		StructuredContent: output,
		IsError:           false,
	}, output, nil
}
