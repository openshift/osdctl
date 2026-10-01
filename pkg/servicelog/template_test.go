package servicelog

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestMessage(description string) Message {
	return Message{
		Severity:    "Info",
		ServiceName: "SREManualAction",
		Summary:     "Test summary",
		Description: description,
	}
}

func TestParseParams(t *testing.T) {
	tests := []struct {
		name    string
		params  []string
		want    map[string]string
		wantErr bool
	}{
		{
			name:   "single param",
			params: []string{"KEY=VAL"},
			want:   map[string]string{"${KEY}": "VAL"},
		},
		{
			name:   "multiple params",
			params: []string{"FOO=bar", "BAZ=qux"},
			want:   map[string]string{"${FOO}": "bar", "${BAZ}": "qux"},
		},
		{
			name:   "value containing equals",
			params: []string{"KEY=VAL=UE"},
			want:   map[string]string{"${KEY}": "VAL=UE"},
		},
		{
			name:   "duplicate keys last wins",
			params: []string{"KEY=first", "KEY=second"},
			want:   map[string]string{"${KEY}": "second"},
		},
		{
			name:    "missing equals",
			params:  []string{"KEYVAL"},
			wantErr: true,
		},
		{
			name:    "empty key",
			params:  []string{"=VAL"},
			wantErr: true,
		},
		{
			name:    "empty value",
			params:  []string{"KEY="},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseParams(tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseParams() got %d entries, want %d", len(got), len(tt.want))
				return
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseParams() got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestFindLeftovers_SubscriptionID(t *testing.T) {
	msg := Message{
		Summary:        "no placeholders here",
		SubscriptionID: "sub-${SUB_ID}",
	}
	matches := msg.FindLeftovers()
	if len(matches) == 0 {
		t.Fatal("expected to find leftover in SubscriptionID")
	}
	if len(matches) != 1 || matches[0] != "${SUB_ID}" {
		t.Errorf("FindLeftovers() = %v, want [${SUB_ID}]", matches)
	}
}

func TestSubstituteParams(t *testing.T) {
	tests := []struct {
		name     string
		msgDesc  string
		params   map[string]string
		wantDesc string
		wantErr  bool
	}{
		{
			name:     "replaces placeholder",
			msgDesc:  "Instance type changed to ${INSTANCE_TYPE}",
			params:   map[string]string{"${INSTANCE_TYPE}": "m5.xlarge"},
			wantDesc: "Instance type changed to m5.xlarge",
		},
		{
			name:     "multiple placeholders",
			msgDesc:  "${TYPE} resized with ${REASON}",
			params:   map[string]string{"${TYPE}": "infra", "${REASON}": "capacity"},
			wantDesc: "infra resized with capacity",
		},
		{
			name:     "CLUSTER_UUID excluded from leftover check",
			msgDesc:  "Cluster ${CLUSTER_UUID} updated",
			params:   map[string]string{},
			wantDesc: "Cluster ${CLUSTER_UUID} updated",
		},
		{
			name:    "CLUSTER_UUID excluded but other placeholder errors",
			msgDesc: "Cluster ${CLUSTER_UUID} has ${MISSING}",
			params:  map[string]string{},
			wantErr: true,
		},
		{
			name:    "unreplaced placeholder errors",
			msgDesc: "Missing ${UNKNOWN}",
			params:  map[string]string{},
			wantErr: true,
		},
		{
			name:    "param not in template errors",
			msgDesc: "No placeholders here",
			params:  map[string]string{"${UNUSED}": "val"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := newTestMessage(tt.msgDesc)
			err := substituteParams(&msg, tt.params)
			if (err != nil) != tt.wantErr {
				t.Errorf("substituteParams() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && msg.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", msg.Description, tt.wantDesc)
			}
		})
	}
}

func TestLoadTemplate_internalOnly(t *testing.T) {
	msg, err := loadTemplate("", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ServiceName != "SREManualAction" {
		t.Errorf("ServiceName = %q, want %q", msg.ServiceName, "SREManualAction")
	}
	if !msg.InternalOnly {
		t.Error("InternalOnly = false, want true")
	}
	if msg.Severity != "Low" {
		t.Errorf("Severity = %q, want %q", msg.Severity, "Low")
	}
	if msg.Description != "${MESSAGE}" {
		t.Errorf("Description = %q, want %q", msg.Description, "${MESSAGE}")
	}
}

func TestLoadTemplate_emptyPathNotInternal(t *testing.T) {
	_, err := loadTemplate("", false)
	if err == nil {
		t.Error("expected error for empty template path with InternalOnly=false")
	}
}

func TestLoadTemplate_fileTemplate(t *testing.T) {
	dir := t.TempDir()
	templateFile := filepath.Join(dir, "test_template.json")
	content := `{"severity":"Info","service_name":"TestService","summary":"test summary","description":"test desc"}`
	if err := os.WriteFile(templateFile, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	msg, err := loadTemplate(templateFile, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Severity != "Info" {
		t.Errorf("Severity = %q, want %q", msg.Severity, "Info")
	}
	if msg.ServiceName != "TestService" {
		t.Errorf("ServiceName = %q, want %q", msg.ServiceName, "TestService")
	}
	if msg.Summary != "test summary" {
		t.Errorf("Summary = %q, want %q", msg.Summary, "test summary")
	}
	if msg.Description != "test desc" {
		t.Errorf("Description = %q, want %q", msg.Description, "test desc")
	}
}

func TestLoadTemplate_invalidJSON(t *testing.T) {
	dir := t.TempDir()
	templateFile := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(templateFile, []byte("not json"), 0600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	_, err := loadTemplate(templateFile, false)
	if err == nil {
		t.Error("expected error for invalid JSON template")
	}
}

func TestLoadTemplate_missingFile(t *testing.T) {
	_, err := loadTemplate("/nonexistent/path/template.json", false)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadTemplate_directory(t *testing.T) {
	dir := t.TempDir()
	_, err := loadTemplate(dir, false)
	if err == nil {
		t.Error("expected error when path is a directory")
	}
}
