package utils

import "testing"

func TestIsManagedInstall(t *testing.T) {
	tests := []struct {
		name          string
		installMethod string
		want          bool
	}{
		{"empty (GitHub release)", "", false},
		{"copr", "copr", true},
		{"homebrew", "homebrew", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := InstallMethod
			defer func() { InstallMethod = original }()
			InstallMethod = tt.installMethod
			if got := IsManagedInstall(); got != tt.want {
				t.Errorf("IsManagedInstall() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpgradeInstruction(t *testing.T) {
	tests := []struct {
		name          string
		installMethod string
		want          string
	}{
		{"copr", "copr", "dnf upgrade osdctl"},
		{"homebrew", "homebrew", "brew upgrade osdctl"},
		{"empty", "", ""},
		{"unknown", "unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := InstallMethod
			defer func() { InstallMethod = original }()
			InstallMethod = tt.installMethod
			if got := UpgradeInstruction(); got != tt.want {
				t.Errorf("UpgradeInstruction() = %q, want %q", got, tt.want)
			}
		})
	}
}
