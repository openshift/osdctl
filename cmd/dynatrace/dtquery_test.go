package dynatrace

import (
	"testing"
	"time"
)

func TestDTQuery_InitLogs(t *testing.T) {
	q := new(DTQuery).InitLogs(2)
	expected := `fetch logs, from:now()-2h 
| filter matchesValue(event.type, "LOG") and `
	if q.fragments[0] != expected {
		t.Errorf("expected: %s\ngot: %s", expected, q.fragments[0])
	}
}

func TestDTQuery_InitEvents(t *testing.T) {
	q := new(DTQuery).InitEvents(4)
	expected := `fetch events, from:now()-4h 
| filter `
	if q.fragments[0] != expected {
		t.Errorf("expected: %s\ngot: %s", expected, q.fragments[0])
	}
}

func TestDTQuery_Cluster(t *testing.T) {
	q := new(DTQuery).InitLogs(1).Cluster("test-cluster")
	expected := `matchesPhrase(dt.kubernetes.cluster.name, "test-cluster")`
	if q.fragments[1] != expected {
		t.Errorf("expected: %s\ngot: %s", expected, q.fragments[1])
	}
}

func TestDTQuery_Namespaces(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Single namespace", []string{"ns1"}, ` and (matchesValue(k8s.namespace.name, "ns1"))`},
		{"Two namespaces", []string{"ns1", "ns2"}, ` and (matchesValue(k8s.namespace.name, "ns1") or matchesValue(k8s.namespace.name, "ns2"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1).Namespaces(tt.input)
			if q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_ContainsPhrase(t *testing.T) {
	q := new(DTQuery).InitLogs(1).ContainsPhrase("error")
	expected := ` and contains(content,"error", caseSensitive:false)`
	if q.fragments[1] != expected {
		t.Errorf("expected: %s\ngot: %s", expected, q.fragments[1])
	}
}

func TestDTQuery_Sort(t *testing.T) {
	tests := []struct {
		name        string
		order       string
		expectError bool
		expected    string
	}{
		{"Valid desc sort", "desc", false, "\n| sort timestamp desc"},
		{"Valid asc sort", "asc", false, "\n| sort timestamp asc"},
		{"Invalid sort", "invalid", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1)
			_, err := q.Sort(tt.order)
			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("did not expect error but got: %v", err)
			}
			if !tt.expectError && q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_Limit(t *testing.T) {
	q := new(DTQuery).InitLogs(1).Limit(50)
	expected := "\n| limit 50"
	if q.fragments[1] != expected {
		t.Errorf("expected: %s\ngot: %s", expected, q.fragments[1])
	}
}

func TestDTQuery_Build(t *testing.T) {
	q := new(DTQuery).
		InitLogs(1).
		Cluster("prod-cluster").
		Namespaces([]string{"ns1"}).
		ContainsPhrase("fail").
		Limit(5)

	expected := `fetch logs, from:now()-1h 
| filter matchesValue(event.type, "LOG") and matchesPhrase(dt.kubernetes.cluster.name, "prod-cluster") and (matchesValue(k8s.namespace.name, "ns1")) and contains(content,"fail", caseSensitive:false)` +
		"\n| limit 5"

	actual := q.Build()
	if actual != expected {
		t.Errorf("expected: %s\ngot: %s", expected, actual)
	}
}

func TestDTQuery_Nodes(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Single node", []string{"node1"}, ` and (matchesValue(k8s.node.name, "node1"))`},
		{"Multiple nodes", []string{"node1", "node2"}, ` and (matchesValue(k8s.node.name, "node1") or matchesValue(k8s.node.name, "node2"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1).Nodes(tt.input)
			if q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_Containers(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Single container", []string{"container1"}, ` and (matchesValue(k8s.container.name, "container1"))`},
		{"Multiple containers", []string{"container1", "container2"}, ` and (matchesValue(k8s.container.name, "container1") or matchesValue(k8s.container.name, "container2"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1).Containers(tt.input)
			if q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_Status(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Single status", []string{"ERROR"}, ` and (matchesValue(status, "ERROR"))`},
		{"Multiple statuses", []string{"ERROR", "INFO"}, ` and (matchesValue(status, "ERROR") or matchesValue(status, "INFO"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1).Status(tt.input)
			if q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_Deployments(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected string
	}{
		{"Single deployment", []string{"api-deploy"}, ` and (matchesValue(dt.kubernetes.workload.name, "api-deploy"))`},
		{"Multiple deployments", []string{"api-deploy", "web-deploy"}, ` and (matchesValue(dt.kubernetes.workload.name, "api-deploy") or matchesValue(dt.kubernetes.workload.name, "web-deploy"))`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogs(1).Deployments(tt.input)
			if q.fragments[1] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[1])
			}
		})
	}
}

func TestDTQuery_InitLogsWithTimeRange(t *testing.T) {
	tests := []struct {
		name     string
		from     time.Time
		to       time.Time
		expected string
	}{
		{
			name: "Standard time range",
			from: time.Date(2025, 6, 12, 5, 0, 0, 0, time.UTC),
			to:   time.Date(2025, 6, 17, 15, 0, 0, 0, time.UTC),
			expected: `fetch logs, from:"2025-06-12T05:00:00Z", to:"2025-06-17T15:00:00Z" 
| filter matchesValue(event.type, "LOG") and `,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := new(DTQuery).InitLogsWithTimeRange(tt.from, tt.to)
			if q.fragments[0] != tt.expected {
				t.Errorf("expected: %s\ngot: %s", tt.expected, q.fragments[0])
			}
		})
	}
}
