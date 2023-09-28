package servicelog

import (
	"encoding/json"
	"fmt"
	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"os"
	"time"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	"github.com/spf13/cobra"
)

const (
	AllMessagesFlag      = "all-messages"
	AllMessagesShortFlag = "A"
	InternalFlag         = "internal"
	InternalShortFlag    = "i"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list [flags] [options] cluster-identifier",
	Short: "gets all servicelog messages for a given cluster",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		allMessages, err := cmd.Flags().GetBool(AllMessagesFlag)
		if err != nil {
			return fmt.Errorf("failed to get flag `--%v`/`-%v`, %w", AllMessagesFlag, AllMessagesShortFlag, err)
		}

		internalOnly, err := cmd.Flags().GetBool(InternalFlag)
		if err != nil {
			return fmt.Errorf("failed to get flag `--%v`/`-%v`, %w", InternalFlag, InternalShortFlag, err)
		}

		return ListServiceLogs(args[0], allMessages, internalOnly)
	},
}

func init() {
	// define flags
	listCmd.Flags().BoolP(AllMessagesFlag, AllMessagesShortFlag, false, "Toggle if we should see all of the messages or only SRE-P specific ones")
	listCmd.Flags().BoolP(InternalFlag, InternalShortFlag, false, "Toggle if we should see internal messages")
}

func ListServiceLogs(clusterID string, allMessages bool, internalOnly bool) error {
	response, err := FetchServiceLogs(clusterID, allMessages, internalOnly)
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
