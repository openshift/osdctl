package servicelog

import "time"

type GoodReply struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Href          string    `json:"href"`
	Timestamp     time.Time `json:"timestamp"`
	Severity      string    `json:"severity"`
	ServiceName   string    `json:"service_name"`
	ClusterUUID   string    `json:"cluster_uuid"`
	Summary       string    `json:"summary"`
	Description   string    `json:"description"`
	EventStreamID string    `json:"event_stream_id"`
	CreatedAt     time.Time `json:"created_at"`
}

type ClusterListGoodReply struct {
	Kind  string `json:"kind"`
	Page  int    `json:"page"`
	Size  int    `json:"size"`
	Total int    `json:"total"`
	Items []struct {
		ExternalID string `json:"external_id"`
	} `json:"items"`
}

type BadReply struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Href        string `json:"href"`
	Code        string `json:"code"`
	Reason      string `json:"reason"`
	OperationID string `json:"operation_id"`
}
