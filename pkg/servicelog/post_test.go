package servicelog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTemplate(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "template.json")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}
	return path
}

func TestPrepare_fileTemplateWithParams(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "Resize to ${INSTANCE_TYPE}",
		"description": "Node resized for ${REASON}"
	}`)

	msg, err := Prepare(PostRequest{
		Template:       path,
		TemplateParams: []string{"INSTANCE_TYPE=m5.xlarge", "REASON=capacity"},
		SkipLinkCheck:  true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if msg.Summary != "Resize to m5.xlarge" {
		t.Errorf("Summary = %q, want %q", msg.Summary, "Resize to m5.xlarge")
	}
	if msg.Description != "Node resized for capacity" {
		t.Errorf("Description = %q, want %q", msg.Description, "Node resized for capacity")
	}
}

func TestPrepare_internalOnly(t *testing.T) {
	msg, err := Prepare(PostRequest{
		InternalOnly:   true,
		TemplateParams: []string{"MESSAGE=test message"},
		SkipLinkCheck:  true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if msg.Severity != "Low" {
		t.Errorf("Severity = %q, want %q", msg.Severity, "Low")
	}
	if !msg.InternalOnly {
		t.Error("InternalOnly = false, want true")
	}
	if msg.Description != "test message" {
		t.Errorf("Description = %q, want %q", msg.Description, "test message")
	}
	if msg.ServiceName != "SREManualAction" {
		t.Errorf("ServiceName = %q, want %q", msg.ServiceName, "SREManualAction")
	}
}

func TestPrepare_noTemplateNotInternal(t *testing.T) {
	_, err := Prepare(PostRequest{SkipLinkCheck: true})
	if err == nil {
		t.Error("expected error when no template and not internal-only")
	}
}

func TestPrepare_missingParam(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "Resize to ${INSTANCE_TYPE}",
		"description": "done"
	}`)

	_, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err == nil {
		t.Error("expected error for unreplaced parameter")
	}
}

func TestPrepare_unusedParam(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "No placeholders",
		"description": "done"
	}`)

	_, err := Prepare(PostRequest{
		Template:       path,
		TemplateParams: []string{"UNUSED=val"},
		SkipLinkCheck:  true,
	})
	if err == nil {
		t.Error("expected error for unused parameter")
	}
}

func TestPrepare_badParamSyntax(t *testing.T) {
	path := writeTemplate(t, `{"severity":"Info","service_name":"S","summary":"s","description":"d"}`)

	_, err := Prepare(PostRequest{
		Template:       path,
		TemplateParams: []string{"NOEQUALSSIGN"},
		SkipLinkCheck:  true,
	})
	if err == nil {
		t.Error("expected error for bad param syntax")
	}
}

func TestPrepare_invalidTemplate(t *testing.T) {
	path := writeTemplate(t, `not json`)

	_, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err == nil {
		t.Error("expected error for invalid JSON template")
	}
}

func TestPrepare_clusterUUIDAllowed(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "Cluster ${CLUSTER_UUID}",
		"description": "done"
	}`)

	msg, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v, ${CLUSTER_UUID} should be allowed", err)
	}
	if msg.Summary != "Cluster ${CLUSTER_UUID}" {
		t.Errorf("Summary = %q, want placeholder preserved", msg.Summary)
	}
}

func TestPrepare_missingFile(t *testing.T) {
	_, err := Prepare(PostRequest{
		Template:      "/nonexistent/template.json",
		SkipLinkCheck: true,
	})
	if err == nil {
		t.Error("expected error for missing template file")
	}
}

func TestPrepare_templateInternalOnlyPreserved(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Warning",
		"service_name": "SREManualAction",
		"summary": "Internal note",
		"description": "Do not share",
		"internal_only": true
	}`)

	msg, err := Prepare(PostRequest{
		Template:      path,
		InternalOnly:  false,
		SkipLinkCheck: true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !msg.InternalOnly {
		t.Error("InternalOnly = false, want true: template's internal_only should be preserved when the flag is not set")
	}
}

func TestPrepare_templateInternalOnlyFalsePreserved(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "External note",
		"description": "Customer-visible",
		"internal_only": false
	}`)

	msg, err := Prepare(PostRequest{
		Template:      path,
		InternalOnly:  false,
		SkipLinkCheck: true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if msg.InternalOnly {
		t.Error("InternalOnly = true, want false: template's internal_only=false should be preserved")
	}
}

func TestPrepare_internalOnlyFlagIgnoresFileTemplate(t *testing.T) {
	msg, err := Prepare(PostRequest{
		Template:       "/should/be/ignored.json",
		InternalOnly:   true,
		TemplateParams: []string{"MESSAGE=secret note"},
		SkipLinkCheck:  true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if !msg.InternalOnly {
		t.Error("InternalOnly = false, want true")
	}
	if msg.Description != "secret note" {
		t.Errorf("Description = %q, want %q", msg.Description, "secret note")
	}
}

func TestPrepare_missingParamNamedInError(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "Version ${VERSION}",
		"description": "done"
	}`)

	_, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err == nil {
		t.Fatal("expected error for unreplaced parameter")
	}
	if !strings.Contains(err.Error(), "${VERSION}") {
		t.Errorf("error should name the missing placeholder, got: %v", err)
	}
}

func TestPrepare_multipleUnreplacedParamsAllNamed(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "Resize ${TYPE} to ${SIZE}",
		"description": "done"
	}`)

	_, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err == nil {
		t.Fatal("expected error for unreplaced parameters")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "${TYPE}") || !strings.Contains(errMsg, "${SIZE}") {
		t.Errorf("error should name all missing placeholders, got: %v", err)
	}
}

func TestPrepare_docReferencesPreserved(t *testing.T) {
	path := writeTemplate(t, `{
		"severity": "Info",
		"service_name": "SREManualAction",
		"summary": "test",
		"description": "test",
		"doc_references": ["https://example.com/doc1", "https://example.com/doc2"]
	}`)

	msg, err := Prepare(PostRequest{
		Template:      path,
		SkipLinkCheck: true,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(msg.DocReferences) != 2 {
		t.Errorf("DocReferences length = %d, want 2", len(msg.DocReferences))
	}
}
