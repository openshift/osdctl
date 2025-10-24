package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/openshift/osdctl/cmd/cluster"
	"github.com/spf13/cobra"
)

type ClusterContextInput struct {
	ClusterId string `json:"cluster_id" jsonschema:"ID of the cluster to retrieve information for"`
}

var ClusterContextInputSchema, _ = jsonschema.For[ClusterContextInput](&jsonschema.ForOptions{})

type ClusterContextOutput struct {
	Context string `json:"context"`
}

var ClusterContextOutputSchema, err = jsonschema.For[ClusterContextOutput](&jsonschema.ForOptions{})

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
		InputSchema:  ClusterContextInputSchema,
		OutputSchema: ClusterContextOutputSchema,
		Title:        "cluster context",
	}, GenerateContext)
	if useHttp {
		// Create the streamable HTTP handler.
		handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
			return server
		}, nil)
		if err := http.ListenAndServe(fmt.Sprintf("http://localhost:%d", httpPort), handler); err != nil {
			return err
		}
	}
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		return err
	}
	return nil
}

func GenerateContext(ctx context.Context, req *mcp.CallToolRequest, input ClusterContextInput) (*mcp.CallToolResult, ClusterContextOutput, error) {
	context, _ := cluster.GenerateContextData(input.ClusterId)
	return nil, ClusterContextOutput{
		context,
	}, nil
}
