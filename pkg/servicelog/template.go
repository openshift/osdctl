package servicelog

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	slv1 "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/openshift/osdctl/internal/utils"
)

type Message struct {
	Severity       string   `json:"severity"`
	ServiceName    string   `json:"service_name"`
	ClusterUUID    string   `json:"cluster_uuid,omitempty"`
	ClusterID      string   `json:"cluster_id,omitempty"`
	Summary        string   `json:"summary"`
	Description    string   `json:"description"`
	InternalOnly   bool     `json:"internal_only"`
	EventStreamID  string   `json:"event_stream_id"`
	SubscriptionID string   `json:"subscription_id,omitempty"`
	DocReferences  []string `json:"doc_references"`
}

func (m *Message) stringFields() []*string {
	return []*string{
		&m.Severity, &m.ServiceName, &m.ClusterUUID, &m.ClusterID,
		&m.Summary, &m.Description, &m.EventStreamID, &m.SubscriptionID,
	}
}

func (m *Message) ReplaceWithFlag(variable, value string) {
	for _, field := range m.stringFields() {
		*field = strings.ReplaceAll(*field, variable, value)
	}
}

func (m *Message) SearchFlag(placeholder string) bool {
	for _, field := range m.stringFields() {
		if strings.Contains(*field, placeholder) {
			return true
		}
	}
	return false
}

func (m *Message) FindLeftovers() []string {
	r := regexp.MustCompile(`\${[^{}]*}`)
	var sb strings.Builder
	for _, field := range m.stringFields() {
		sb.WriteString(*field)
	}
	return r.FindAllString(sb.String(), -1)
}

// InternalOnlyTemplate is the fixed JSON template used for internal-only
// service logs (the -i flag in the CLI).
const InternalOnlyTemplate = `{
	"service_name": "SREManualAction",
	"summary": "INTERNAL ONLY, DO NOT SHARE WITH CUSTOMER",
	"description": "${MESSAGE}",
	"internal_only": true
}`

// loadTemplate loads a service log template from a URL, file path, or the
// hardcoded internal-only template. Returns the parsed Message.
func loadTemplate(templatePath string, internalOnly bool) (Message, error) {
	var msg Message

	if internalOnly {
		if err := json.Unmarshal([]byte(InternalOnlyTemplate), &msg); err != nil {
			return msg, fmt.Errorf("cannot parse internal message template: %w", err)
		}
		msg.Severity = string(slv1.SeverityLow)
		return msg, nil
	}

	if templatePath == "" {
		return msg, fmt.Errorf("template is required when not using internal-only mode")
	}

	data, err := utils.AccessFile(templatePath)
	if err != nil {
		return msg, err
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		return msg, fmt.Errorf("cannot parse JSON template from %q: %w", templatePath, err)
	}

	return msg, nil
}

// parseParams splits KEY=VAL strings into a map of "${KEY}" -> "VAL".
func parseParams(params []string) (map[string]string, error) {
	result := make(map[string]string, len(params))
	for _, v := range params {
		if !strings.Contains(v, "=") {
			return nil, fmt.Errorf("wrong parameter syntax %q: expected KEY=VAL", v)
		}
		param := strings.SplitN(v, "=", 2)
		if param[0] == "" || param[1] == "" {
			return nil, fmt.Errorf("wrong parameter syntax %q: expected KEY=VAL", v)
		}
		result[fmt.Sprintf("${%s}", param[0])] = param[1]
	}
	return result, nil
}

// substituteParams replaces ${KEY} placeholders in the message with values
// from the params map. Returns an error if any placeholders remain
// (excluding ${CLUSTER_UUID} which is set per-cluster at send time).
func substituteParams(msg *Message, params map[string]string) error {
	for key, val := range params {
		if !msg.SearchFlag(key) {
			return fmt.Errorf("template does not use parameter %q", key)
		}
		msg.ReplaceWithFlag(key, val)
	}

	leftovers := msg.FindLeftovers()
	if len(leftovers) == 0 {
		return nil
	}

	var unreplaced []string
	for _, l := range leftovers {
		if l != "${CLUSTER_UUID}" {
			unreplaced = append(unreplaced, l)
		}
	}
	if len(unreplaced) > 0 {
		return fmt.Errorf("template has unreplaced parameters: %s", strings.Join(unreplaced, ", "))
	}

	return nil
}
