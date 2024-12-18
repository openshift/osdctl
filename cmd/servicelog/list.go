package servicelog

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	"github.com/spf13/cobra"
)

const (
	AllMessagesFlag      = "all-messages"
	AllMessagesShortFlag = "A"
	InternalFlag         = "internal"
	InternalShortFlag    = "i"
)

type listCmdOptions struct {
	allMessages bool
	internal    bool
}

func newListCmd() *cobra.Command {
	opts := &listCmdOptions{}
	cmd := &cobra.Command{
		Use: "list [flags] [options] cluster-identifier",
		Long: `Get service logs for a given cluster identifier.

# To return just service logs created by SREs
osdctl servicelog list

# To return all service logs, including those by automated systems
osdctl servicelog list --all-messages

# To return all service logs, as well as internal service logs
osdctl servicelog list --all-messages --internal
`,
		Short: "Get service logs for a given cluster identifier.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return listServiceLogs(args[0], opts)
		},
	}

	cmd.Flags().BoolP("all-messages", "A", opts.allMessages, "Toggle if we should see all of the messages or only SRE-P specific ones")
	cmd.Flags().BoolP("internal", "i", opts.internal, "Toggle if we should see internal messages")

	return cmd
}

func listServiceLogs(clusterID string, opts *listCmdOptions) error {
	response, err := FetchServiceLogs(clusterID, opts.allMessages, opts.internal)
	if err != nil {
		return fmt.Errorf("failed to fetch service logs: %w", err)
	}

	if err = printServiceLogResponse(response); err != nil {
		return fmt.Errorf("failed to print service logs: %w", err)
	}

	return nil
}

func printServiceLogResponse(response *slv1.ClustersClusterLogsListResponse) error {
	entryViews := logEntryToView(response.Items().Slice())
	slices.Reverse(entryViews)
	view := LogEntryResponseView{
		Items: entryViews,
		Kind:  "ClusterLogList",
		Page:  response.Page(),
		Size:  response.Size(),
		Total: response.Total(),
	}

	viewBytes, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("failed to marshal response for output: %w", err)
	}

	return dump.Pretty(os.Stdout, viewBytes)
}

type LogEntryResponseView struct {
	Items []*LogEntryView `json:"items"`
	Kind  string          `json:"kind"`
	Page  int             `json:"page"`
	Size  int             `json:"size"`
	Total int             `json:"total"`
}

type LogEntryView struct {
	ClusterID     string    `json:"cluster_id"`
	ClusterUUID   string    `json:"cluster_uuid"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
	Description   string    `json:"description"`
	DocReferences []string  `json:"doc_references"`
	EventStreamID string    `json:"event_stream_id"`
	Href          string    `json:"href"`
	ID            string    `json:"id"`
	InternalOnly  bool      `json:"internal_only"`
	Kind          string    `json:"kind"`
	LogType       string    `json:"log_type"`
	ServiceName   string    `json:"service_name"`
	Severity      string    `json:"severity"`
	Summary       string    `json:"summary"`
	Timestamp     time.Time `json:"timestamp"`
	Username      string    `json:"username"`
}

func logEntryToView(entries []*slv1.LogEntry) []*LogEntryView {
	entryViews := make([]*LogEntryView, 0, len(entries))
	for _, entry := range entries {
		entryView := &LogEntryView{
			ClusterID:     entry.ClusterID(),
			ClusterUUID:   entry.ClusterUUID(),
			CreatedAt:     entry.CreatedAt(),
			CreatedBy:     entry.CreatedBy(),
			Description:   entry.Description(),
			DocReferences: entry.DocReferences(),
			EventStreamID: entry.EventStreamID(),
			Href:          entry.HREF(),
			ID:            entry.ID(),
			InternalOnly:  entry.InternalOnly(),
			Kind:          entry.Kind(),
			LogType:       string(entry.LogType()),
			ServiceName:   entry.ServiceName(),
			Severity:      string(entry.Severity()),
			Summary:       entry.Summary(),
			Timestamp:     entry.Timestamp(),
			Username:      entry.Username(),
		}
		entryViews = append(entryViews, entryView)
	}
	return entryViews
}
