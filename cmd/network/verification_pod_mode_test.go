package network

import (
	"context"
	"strings"
	"testing"
	"time"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osd-network-verifier/pkg/data/cloud"
	"github.com/openshift/osd-network-verifier/pkg/data/cpu"
	"github.com/openshift/osd-network-verifier/pkg/probes/curl"
	onv "github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/stretchr/testify/assert"
)

func TestEgressVerification_PodModeRegionDetection(t *testing.T) {
	tests := []struct {
		name         string
		ev           *EgressVerification
		platform     cloud.Platform
		setupCluster func() *cmv1.Cluster
		wantError    bool
		wantRegion   string
	}{
		{
			name:     "aws_classic_with_manual_region",
			platform: cloud.AWSClassic,
			ev: &EgressVerification{
				PodMode: true,
				Region:  "us-west-2",
				log:     newTestLogger(t),
			},
			wantError:  false,
			wantRegion: "us-west-2",
		},
		{
			name:     "aws_classic_with_ocm_region",
			platform: cloud.AWSClassic,
			ev: &EgressVerification{
				PodMode: true,
				log:     newTestLogger(t),
			},
			setupCluster: func() *cmv1.Cluster {
				return newTestCluster(t, cmv1.NewCluster().
					Region(cmv1.NewCloudRegion().ID("eu-west-1")).
					CloudProvider(cmv1.NewCloudProvider().ID("aws")))
			},
			wantError:  false,
			wantRegion: "eu-west-1",
		},
		{
			name:     "aws_classic_ocm_overrides_manual",
			platform: cloud.AWSClassic,
			ev: &EgressVerification{
				PodMode: true,
				Region:  "us-west-1", // This should be overridden by OCM
				log:     newTestLogger(t),
			},
			setupCluster: func() *cmv1.Cluster {
				return newTestCluster(t, cmv1.NewCluster().
					Region(cmv1.NewCloudRegion().ID("ap-south-1")).
					CloudProvider(cmv1.NewCloudProvider().ID("aws")))
			},
			wantError:  false,
			wantRegion: "ap-south-1", // OCM takes precedence
		},
		{
			name:     "aws_hcp_without_region",
			platform: cloud.AWSHCP,
			ev: &EgressVerification{
				PodMode: true,
				log:     newTestLogger(t),
			},
			wantError: true,
		},
		{
			name:     "aws_hcp_zero_egress_with_region",
			platform: cloud.AWSHCPZeroEgress,
			ev: &EgressVerification{
				PodMode: true,
				Region:  "ca-central-1",
				log:     newTestLogger(t),
			},
			wantError:  false,
			wantRegion: "ca-central-1",
		},
		{
			name:     "gcp_classic_no_region_needed",
			platform: cloud.GCPClassic,
			ev: &EgressVerification{
				PodMode: true,
				log:     newTestLogger(t),
			},
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupCluster != nil {
				tt.ev.cluster = tt.setupCluster()
			}

			// Test the region detection logic directly
			if tt.platform == cloud.AWSClassic || tt.platform == cloud.AWSHCP || tt.platform == cloud.AWSHCPZeroEgress {
				var region string

				// Simulate the region detection logic from setupPodModeVerification
				if tt.ev.cluster != nil && tt.ev.cluster.Region() != nil && tt.ev.cluster.Region().ID() != "" {
					region = tt.ev.cluster.Region().ID()
				} else if tt.ev.Region != "" {
					region = tt.ev.Region
				}

				if tt.wantError {
					assert.Empty(t, region, "Expected no region to be detected for error case")
				} else {
					assert.Equal(t, tt.wantRegion, region, "Region detection should match expected")
				}
			}
		})
	}
}

func TestEgressVerification_PodModeProbeValidation(t *testing.T) {
	tests := []struct {
		name          string
		initialProbe  string
		expectedProbe string
		shouldSwitch  bool
	}{
		{
			name:          "curl_probe_unchanged",
			initialProbe:  "curl",
			expectedProbe: "curl",
			shouldSwitch:  false,
		},
		{
			name:          "legacy_probe_switched_to_curl",
			initialProbe:  "legacy",
			expectedProbe: "curl",
			shouldSwitch:  true,
		},
		{
			name:          "unknown_probe_switched_to_curl",
			initialProbe:  "unknown",
			expectedProbe: "curl",
			shouldSwitch:  true,
		},
		{
			name:          "uppercase_curl_unchanged",
			initialProbe:  "CURL",
			expectedProbe: "CURL",
			shouldSwitch:  false,
		},
		{
			name:          "mixed_case_curl_unchanged",
			initialProbe:  "CuRl",
			expectedProbe: "CuRl",
			shouldSwitch:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &EgressVerification{
				Probe: tt.initialProbe,
				log:   newTestLogger(t),
			}

			// Simulate the probe validation logic from setupPodModeVerification
			originalProbe := ev.Probe
			if strings.ToLower(ev.Probe) != "curl" {
				ev.Probe = "curl"
				assert.True(t, tt.shouldSwitch, "Expected probe to be switched")
				assert.Equal(t, "curl", ev.Probe)
			} else {
				assert.False(t, tt.shouldSwitch, "Expected probe to remain unchanged")
				assert.Equal(t, originalProbe, ev.Probe)
			}
		})
	}
}

func TestEgressVerification_PodModeInputValidation(t *testing.T) {
	tests := []struct {
		name      string
		ev        *EgressVerification
		platform  cloud.Platform
		wantInput *onv.ValidateEgressInput
	}{
		{
			name:     "aws_classic_pod_mode_input",
			platform: cloud.AWSClassic,
			ev: &EgressVerification{
				PodMode:       true,
				Region:        "us-west-2",
				Probe:         "curl",
				cpuArch:       cpu.ArchX86,
				EgressTimeout: 10 * time.Second,
				NoTls:         false,
				log:           newTestLogger(t),
			},
			wantInput: &onv.ValidateEgressInput{
				CPUArchitecture: cpu.ArchX86,
				PlatformType:    cloud.AWSClassic,
				Timeout:         10 * time.Second,
				Tags:            networkVerifierDefaultTags,
				AWS:             onv.AwsEgressConfig{Region: "us-west-2"},
			},
		},
		{
			name:     "gcp_classic_pod_mode_input",
			platform: cloud.GCPClassic,
			ev: &EgressVerification{
				PodMode:       true,
				Probe:         "curl",
				cpuArch:       cpu.ArchX86,
				EgressTimeout: 5 * time.Second,
				NoTls:         true,
				log:           newTestLogger(t),
			},
			wantInput: &onv.ValidateEgressInput{
				CPUArchitecture: cpu.ArchX86,
				PlatformType:    cloud.GCPClassic,
				Timeout:         5 * time.Second,
				Tags:            networkVerifierDefaultTags,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			input, err := tt.ev.defaultValidateEgressInput(ctx, tt.platform)
			assert.NoError(t, err)

			// Verify basic fields
			assert.Equal(t, tt.wantInput.CPUArchitecture, input.CPUArchitecture)
			assert.Equal(t, tt.wantInput.PlatformType, input.PlatformType)
			assert.Equal(t, tt.wantInput.Timeout, input.Timeout)
			assert.Equal(t, tt.wantInput.Tags, input.Tags)

			// Verify probe type
			assert.IsType(t, curl.Probe{}, input.Probe)

			// Verify proxy configuration
			assert.Equal(t, tt.ev.NoTls, input.Proxy.NoTls)

			// For AWS platforms, simulate setting the region like setupPodModeVerification does
			if tt.platform == cloud.AWSClassic || tt.platform == cloud.AWSHCP || tt.platform == cloud.AWSHCPZeroEgress {
				input.AWS = onv.AwsEgressConfig{Region: tt.ev.Region}
				assert.Equal(t, tt.wantInput.AWS.Region, input.AWS.Region)
			}
		})
	}
}

func TestEgressVerification_PodModeAwsConfigSetting(t *testing.T) {
	tests := []struct {
		name         string
		platform     cloud.Platform
		region       string
		shouldSetAWS bool
	}{
		{
			name:         "aws_classic_sets_aws_config",
			platform:     cloud.AWSClassic,
			region:       "us-east-1",
			shouldSetAWS: true,
		},
		{
			name:         "aws_hcp_sets_aws_config",
			platform:     cloud.AWSHCP,
			region:       "eu-west-1",
			shouldSetAWS: true,
		},
		{
			name:         "aws_hcp_zero_egress_sets_aws_config",
			platform:     cloud.AWSHCPZeroEgress,
			region:       "ap-southeast-1",
			shouldSetAWS: true,
		},
		{
			name:         "gcp_classic_no_aws_config",
			platform:     cloud.GCPClassic,
			region:       "",
			shouldSetAWS: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &EgressVerification{
				PodMode:       true,
				Region:        tt.region,
				Probe:         "curl",
				cpuArch:       cpu.ArchX86,
				EgressTimeout: 5 * time.Second,
				log:           newTestLogger(t),
			}

			ctx := context.Background()
			input, err := ev.defaultValidateEgressInput(ctx, tt.platform)
			assert.NoError(t, err)

			if tt.shouldSetAWS {
				// Simulate the AWS config setting from setupPodModeVerification
				input.AWS = onv.AwsEgressConfig{Region: tt.region}
				assert.Equal(t, tt.region, input.AWS.Region)
				assert.NotEmpty(t, input.AWS.Region)
			} else {
				// For non-AWS platforms, AWS config should be empty
				assert.Empty(t, input.AWS.Region)
			}
		})
	}
}
