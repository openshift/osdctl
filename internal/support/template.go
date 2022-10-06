package support

import (
	"strings"
	"time"
)

// GoodReply is the template for good reply
type GoodReply struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	Href              string    `json:"href"`
	Details           string    `json:"details"`
	DetectionType     string    `json:"detection_type"`
	Summary           string    `json:"summary"`
	CreationTimestamp time.Time `json:"creation_timestamp"`
}

// BadReply is the template for bad reply
type BadReply struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Href    string `json:"href"`
	Code    string `json:"code"`
	Reason  string `json:"reason"`
	Details []struct {
		Description string `json:"description"`
	} `json:"details"`
}

// LimitedSupport is the base limited_support_reasons template
type LimitedSupport struct {
	ID            string `json:"id,omitempty"`
	TemplateID    string `json:"template_id,omitempty"`
	Summary       string `json:"summary"`
	Details       string `json:"details"`
	DetectionType string `json:"detection_type"`
}

func (l *LimitedSupport) GetSummary() string {
	return l.Summary
}

func (l *LimitedSupport) GetDescription() string {
	return l.Details
}

func (l *LimitedSupport) ReplaceWithFlag(variable, value string) {
	l.Summary = strings.ReplaceAll(l.Summary, variable, value)
	l.Details = strings.ReplaceAll(l.Details, variable, value)
}

func (l *LimitedSupport) SearchFlag(placeholder string) (found bool) {
	if found = strings.Contains(l.Summary, placeholder); found {
		return found
	}
	if found = strings.Contains(l.Details, placeholder); found {
		return found
	}
	return false
}
