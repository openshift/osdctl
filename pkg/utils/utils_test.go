package utils

import (
	"runtime/debug"
	"testing"
)

func mockReadBuildInfo(parseBuildInfoError bool) func() (info *debug.BuildInfo, ok bool) {
	return func() (*debug.BuildInfo, bool) {
		info := &debug.BuildInfo{
			Deps: []*debug.Module{
				{
					Path:    "foo",
					Version: "v1.2.3",
				},
				{
					Path:    "bar",
					Version: "v4.5.6",
				},
			},
		}
		return info, !parseBuildInfoError
	}
}

func TestGetDependencyVersion(t *testing.T) {
	tests := []struct {
		name                string
		dependencyPath      string
		parseBuildInfoError bool
		want                string
		wantErr             bool
	}{
		{
			name:                "Error parsing build info",
			parseBuildInfoError: true,
			wantErr:             true,
		},
		{
			name:           "Dependency not found",
			dependencyPath: "test",
			wantErr:        true,
		},
		{
			name:           "Finds and returns version successfully (1)",
			dependencyPath: "foo",
			want:           "v1.2.3",
		},
		{
			name:           "Finds and returns version successfully (2)",
			dependencyPath: "bar",
			want:           "v4.5.6",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ReadBuildInfo = mockReadBuildInfo(tt.parseBuildInfoError)
			got, err := GetDependencyVersion(tt.dependencyPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetDependencyVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetDependencyVersion() got = %v, want %v", got, tt.want)
			}
		})
	}
}
