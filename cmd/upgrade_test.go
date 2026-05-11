package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openshift/osdctl/pkg/utils"
)

func TestUpgradeRefusesWhenManaged(t *testing.T) {
	tests := []struct {
		name          string
		installMethod string
		wantSubstring string
	}{
		{"copr", "copr", "dnf upgrade osdctl"},
		{"homebrew", "homebrew", "brew upgrade osdctl"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := utils.InstallMethod
			defer func() { utils.InstallMethod = original }()
			utils.InstallMethod = tt.installMethod

			var buf bytes.Buffer
			upgradeCmd.SetErr(&buf)
			defer upgradeCmd.SetErr(nil)

			err := upgrade(upgradeCmd, nil)
			if err != nil {
				t.Fatalf("expected nil error, got: %v", err)
			}
			output := buf.String()
			if !strings.Contains(output, tt.installMethod) {
				t.Errorf("output should mention %q, got: %s", tt.installMethod, output)
			}
			if !strings.Contains(output, tt.wantSubstring) {
				t.Errorf("output should contain %q, got: %s", tt.wantSubstring, output)
			}
		})
	}
}
