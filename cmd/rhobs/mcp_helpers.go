package rhobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/sync/singleflight"
)

var fetcherCache sync.Map
var fetcherInit singleflight.Group

func quickVaultCheck() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "vault", "token", "lookup")
	cmd.Env = append(os.Environ(), "VAULT_ADDR=https://vault.devshift.net")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("vault CLI not found in PATH; install Vault and retry")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("vault token lookup timed out; verify VAULT_ADDR and network connectivity")
		}
		return fmt.Errorf("vault token expired or missing. Run: VAULT_ADDR=https://vault.devshift.net vault login -method=oidc")
	}
	return nil
}

func getCachedFetcher(clusterId string, usage RhobsFetchUsage) (*RhobsFetcher, error) {
	key := fmt.Sprintf("%s:%s", clusterId, usage)
	if cached, ok := fetcherCache.Load(key); ok {
		return cached.(*RhobsFetcher), nil
	}

	v, err, _ := fetcherInit.Do(key, func() (interface{}, error) {
		if cached, ok := fetcherCache.Load(key); ok {
			return cached, nil
		}
		if err := quickVaultCheck(); err != nil {
			return nil, err
		}
		fetcher, err := CreateRhobsFetcher(clusterId, usage, commonOptions.hiveOcmUrl)
		if err != nil {
			return nil, err
		}
		actual, _ := fetcherCache.LoadOrStore(key, fetcher)
		return actual, nil
	})
	if err != nil {
		return nil, err
	}
	return v.(*RhobsFetcher), nil
}

func mcpResultJSON(data interface{}) (*mcp.CallToolResult, error) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %v", err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(jsonData)}},
		// Because a JSON output schema is defined clients, e.g. opencode, expecting
		// structured content will fail unless it is returned in the JSON RPC response
		// alongside the text content
		StructuredContent: data,
	}, nil
}

func mcpError(format string, args ...interface{}) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf(format, args...)}},
		IsError: true,
	}, nil
}

func boolPtr(b bool) *bool {
	return &b
}
