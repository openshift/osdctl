package servicelog

import (
	"fmt"
	"regexp"
	"strings"

	ocm_servicelog "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"
	"github.com/pkg/errors"
)

// Message is the base template structure
type Message struct {
	Builder *ocm_servicelog.LogEntryBuilder
	*ocm_servicelog.LogEntry
}

func NewMessage() Message {
	return Message{
		Builder: ocm_servicelog.NewLogEntry(),
	}
}

func (m *Message) ReplaceWithFlag(variable, value string) error {
	m.Builder = m.Builder.Copy(m.LogEntry)
	m.Builder = m.Builder.Severity(ocm_servicelog.Severity(strings.ReplaceAll(string(m.LogEntry.Severity()), variable, value)))
	m.Builder = m.Builder.ServiceName(strings.ReplaceAll(m.LogEntry.ServiceName(), variable, value))
	m.Builder = m.Builder.ClusterUUID(strings.ReplaceAll(m.LogEntry.ClusterUUID(), variable, value))
	m.Builder = m.Builder.Summary(strings.ReplaceAll(m.LogEntry.Summary(), variable, value))
	m.Builder = m.Builder.Description(strings.ReplaceAll(m.LogEntry.Description(), variable, value))
	m.Builder = m.Builder.EventStreamID(strings.ReplaceAll(m.LogEntry.EventStreamID(), variable, value))
	if err := m.Refresh(); err != nil {
		return errors.Wrap(err, "could not refresh after replacing flags")
	}
	return nil
}

func (m Message) SearchFlag(placeholder string) (found bool) {
	if found = strings.Contains(string(m.Severity()), placeholder); found {
		return found
	}
	if found = strings.Contains(m.ServiceName(), placeholder); found {
		return found
	}
	if found = strings.Contains(m.ClusterUUID(), placeholder); found {
		return found
	}
	if found = strings.Contains(m.Summary(), placeholder); found {
		return found
	}
	if found = strings.Contains(m.Description(), placeholder); found {
		return found
	}
	if found = strings.Contains(m.EventStreamID(), placeholder); found {
		return found
	}
	if found = strings.Contains(m.SubscriptionID(), placeholder); found {
		return found
	}
	return false
}

func (m Message) FindLeftovers() (matches []string, found bool) {
	r := regexp.MustCompile(`\${[^{}]*}`)
	str := string(m.Severity()) + m.ServiceName() + m.ClusterUUID() + m.Summary() + m.Description() + m.EventStreamID()
	matches = r.FindAllString(str, -1)
	if len(matches) > 0 {
		found = true
	}
	return matches, found
}
func (m *Message) Refresh() error {
	if logEntry, err := m.Builder.Build(); err != nil {
		return fmt.Errorf("cannot build LogEntry: %v", err)
	} else {
		m.LogEntry = logEntry
	}
	return nil
}
