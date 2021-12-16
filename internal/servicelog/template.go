package servicelog

import (
	"regexp"
	"strings"
)

// Message is the base template structure
type Message struct {
	Severity      string `json:"severity"`
	ServiceName   string `json:"service_name"`
	ClusterUUID   string `json:"cluster_uuid,omitempty"`
	ClusterID     string `json:"cluster_id,omitempty"`
	Summary       string `json:"summary"`
	Description   string `json:"description"`
	InternalOnly  bool   `json:"internal_only"`
	EventStreamID string `json:"event_stream_id"`
}

func (m *Message) GetSeverity() string {
	return m.Severity
}

func (m *Message) GetServiceName() string {
	return m.ServiceName
}

func (m *Message) GetClusterUUID() string {
	return m.ClusterUUID
}

func (m *Message) GetClusterID() string {
	return m.ClusterID
}

func (m *Message) GetSummary() string {
	return m.Summary
}

func (m *Message) GetDescription() string {
	return m.Description
}

func (m *Message) GetInternalOnly() bool {
	return m.InternalOnly
}

func (m *Message) GetEventStreamID() string {
	return m.EventStreamID
}

func (m *Message) ReplaceWithFlag(variable, value string) {
	m.Severity = strings.ReplaceAll(m.Severity, variable, value)
	m.ServiceName = strings.ReplaceAll(m.ServiceName, variable, value)
	m.ClusterUUID = strings.ReplaceAll(m.ClusterUUID, variable, value)
	m.ClusterID = strings.ReplaceAll(m.ClusterID, variable, value)
	m.Summary = strings.ReplaceAll(m.Summary, variable, value)
	m.Description = strings.ReplaceAll(m.Description, variable, value)
	m.EventStreamID = strings.ReplaceAll(m.EventStreamID, variable, value)
}

func (m *Message) SearchFlag(placeholder string) (found bool) {
	if found = strings.Contains(m.Severity, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.ServiceName, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.ClusterUUID, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.ClusterID, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.Summary, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.Description, placeholder); found == true {
		return found
	}
	if found = strings.Contains(m.EventStreamID, placeholder); found == true {
		return found
	}
	return false
}

func (m *Message) FindLeftovers() (matches []string, found bool) {
	r := regexp.MustCompile(`\${[^{}]*}`)
	str := m.Severity + m.ServiceName + m.ClusterUUID + m.Summary + m.Description + m.EventStreamID
	matches = r.FindAllString(str, -1)
	if len(matches) > 0 {
		found = true
	}
	return matches, found
}
