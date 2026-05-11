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
		wantErr       bool
	}{
		{"copr", "copr", "dnf upgrade osdctl", false},
		{"homebrew", "homebrew", "brew upgrade osdctl", false},
		{"empty", "", "", false},
		{"unknown", "unknown", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := InstallMethod
			defer func() { InstallMethod = original }()
			InstallMethod = tt.installMethod
			got, err := UpgradeInstruction()
			if (err != nil) != tt.wantErr {
				t.Errorf("UpgradeInstruction() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("UpgradeInstruction() = %q, want %q", got, tt.want)
			}
		})
	}
}
