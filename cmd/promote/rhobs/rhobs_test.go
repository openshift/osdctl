package rhobs

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openshift/osdctl/pkg/promote"
)

const testSaasYAML = `---
$schema: /app-sre/saas-file-2.yml

labels:
  service: observatorium

name: rhobs-hcp-rules
displayName: rhobs-hcp-rules
description: SaaS tracking file for RHOBS HCP tenant rules

app:
  $ref: /services/rhobs/observatorium-mst/app.yml

resourceTemplates:
- name: rhobs-hcp-rules-rhobsi01uw2
  path: /resources/tenant-rules/hcp.yaml
  url: https://gitlab.cee.redhat.com/rhobs/configuration
  targets:
  - namespace:
      $ref: /services/rhobs/rhobs/namespaces/rhobsi01uw2/rhobs-integration.yml
    ref: main
    parameters:
      NAMESPACE: rhobs-integration

- name: rhobs-hcp-rules-rhobsp01ue1
  path: /resources/tenant-rules/hcp.yaml
  url: https://gitlab.cee.redhat.com/rhobs/configuration
  targets:
  - namespace:
      $ref: /services/rhobs/rhobs/namespaces/rhobsp01ue1/rhobs-production.yml
    ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    parameters:
      NAMESPACE: rhobs-production
      TENANT: EFD08939-FE1D-41A1-A28A-BE9A9BC68003

- name: rhobs-hcp-rules-rhobsp01uw2
  path: /resources/tenant-rules/hcp.yaml
  url: https://gitlab.cee.redhat.com/rhobs/configuration
  targets:
  - namespace:
      $ref: /services/rhobs/rhobs/namespaces/rhobsp01uw2/rhobs-production.yml
    ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
    parameters:
      NAMESPACE: rhobs-production
      TENANT: EFD08939-FE1D-41A1-A28A-BE9A9BC68003
`

const testAppYAML = `---
$schema: /app-sre/app-1.yml
labels: {}
name: observatorium-mst
description: Observatorium MST
codeComponents:
- name: configuration
  resource: upstream
  url: https://gitlab.cee.redhat.com/rhobs/configuration
`

func setupTestService(t *testing.T) (*promote.Service, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create saas file
	saasDir := filepath.Join(tmpDir, "data", "services", "rhobs", "rhobs", "cicd")
	if err := os.MkdirAll(saasDir, 0755); err != nil {
		t.Fatal(err)
	}
	saasPath := filepath.Join(saasDir, "saas-hcp-rules.yaml")
	if err := os.WriteFile(saasPath, []byte(testSaasYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Create app.yml referenced by $ref
	appDir := filepath.Join(tmpDir, "data", "services", "rhobs", "observatorium-mst")
	if err := os.MkdirAll(appDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appDir, "app.yml"), []byte(testAppYAML), 0600); err != nil {
		t.Fatal(err)
	}

	// Initialize git repo with app-interface remote
	for _, args := range [][]string{
		{"init", tmpDir},
		{"-C", tmpDir, "remote", "add", "upstream", "git@gitlab.cee.redhat.com:service/app-interface.git"},
		{"-C", tmpDir, "add", "."},
	} {
		cmd := exec.Command("git", args...)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v failed: %v", args, err)
		}
	}
	cmd := exec.Command("git", "-C", tmpDir, "commit", "-m", "init")
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git commit failed: %v", err)
	}

	appInterfaceClone, err := promote.FindAppInterfaceClone(tmpDir)
	if err != nil {
		t.Fatalf("FindAppInterfaceClone: %v", err)
	}

	svc, err := promote.ReadServiceFromFile(appInterfaceClone, saasPath)
	if err != nil {
		t.Fatalf("ReadServiceFromFile: %v", err)
	}

	return svc, saasPath
}

func TestUpdateProductionTargets(t *testing.T) {
	svc, filePath := setupTestService(t)

	newHash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	updated, err := updateProductionTargets(svc, newHash)
	if err != nil {
		t.Fatalf("updateProductionTargets failed: %v", err)
	}
	if !updated {
		t.Fatal("expected targets to be updated")
	}

	result, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	resultStr := string(result)

	if !strings.Contains(resultStr, "ref: "+newHash) {
		t.Error("new hash not found in output")
	}
	if strings.Contains(resultStr, "ref: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Error("old hash still present in output")
	}

	// Integration target should NOT be modified
	if !strings.Contains(resultStr, "ref: main") {
		t.Error("integration ref was incorrectly modified")
	}
}

func TestUpdateProductionTargetsNoChange(t *testing.T) {
	svc, filePath := setupTestService(t)

	original, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read original file: %v", err)
	}

	updated, err := updateProductionTargets(svc, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatalf("updateProductionTargets failed: %v", err)
	}
	if updated {
		t.Error("expected no update when hash is the same")
	}

	after, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read file after update: %v", err)
	}
	if string(original) != string(after) {
		t.Error("file was modified despite no hash change")
	}
}

func TestIsPinnedSHA(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"6feece228fd187c19c02acf731e891f87bc6f8ad", true},
		{"main", false},
		{"short", false},
		{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isPinnedSHA(tt.input); got != tt.expected {
				t.Errorf("isPinnedSHA(%q) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}
