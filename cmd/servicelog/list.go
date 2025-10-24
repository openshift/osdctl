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

type listCmdOptions struct {
	allMessages bool
	internal    bool
	clusterID   string
}

func newListCmd() *cobra.Command {
	opts := &listCmdOptions{}
	cmd := &cobra.Command{
		Use: "list --cluster-id <cluster-identifier> [flags] [options]",
		Long: `Get service logs for a given cluster identifier.

# To return just service logs created by SREs
osdctl servicelog list --cluster-id=my-cluster-id

# To return all service logs, including those by automated systems
osdctl servicelog list --cluster-id=my-cluster-id --all-messages

# To return all service logs, as well as internal service logs
osdctl servicelog list --cluster-id=my-cluster-id --all-messages --internal
`,
		Short: "Get service logs for a given cluster identifier.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return listServiceLogs(opts.clusterID, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.allMessages, "all-messages", "A", false, "Toggle if we should see all of the messages or only SRE-P specific ones")
	cmd.Flags().BoolVarP(&opts.internal, "internal", "i", false, "Toggle if we should see internal messages")
	cmd.Flags().StringVarP(&opts.clusterID, "cluster-id", "C", "", "Internal Cluster identifier (required)")
	_ = cmd.MarkFlagRequired("cluster-id")

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
	view := ConvertOCMSlToLogEntryView(response)

	viewBytes, err := json.Marshal(view)
	if err != nil {
		return fmt.Errorf("failed to marshal response for output: %w", err)
	}

	return dump.Pretty(os.Stdout, viewBytes)
}

func ConvertOCMSlToLogEntryView(response *slv1.ClustersClusterLogsListResponse) LogEntryResponseView {
	entryViews := logEntryToView(response.Items().Slice())
	slices.Reverse(entryViews)
	view := LogEntryResponseView{
		Items: entryViews,
		Kind:  "ClusterLogList",
		Page:  response.Page(),
		Size:  response.Size(),
		Total: response.Total(),
	}
	return view
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
	// Forces an empty array to actual be [] when Marshalled and not null - this is a JSONSCHEMA error that is
	// configurable json v2: https://pkg.go.dev/encoding/json/v2#FormatNilSliceAsNull
	emptyDocReference := []string{}
	entryViews := make([]*LogEntryView, 0, len(entries))
	for _, entry := range entries {
		var docRef []string
		if len(entry.DocReferences()) > 0 {
			docRef = entry.DocReferences()
		} else {
			docRef = emptyDocReference
		}
		entryView := &LogEntryView{
			ClusterID:     entry.ClusterID(),
			ClusterUUID:   entry.ClusterUUID(),
			CreatedAt:     entry.CreatedAt(),
			CreatedBy:     entry.CreatedBy(),
			Description:   entry.Description(),
			DocReferences: docRef,
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
