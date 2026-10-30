package cmd

import internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"

type ClusterInfo = internalclient.ClusterInfo
type ServiceClusterInfo = internalclient.ServiceClusterInfo
type ManagementClusterClient = internalclient.ManagementClusterClient
type ManagementClusterOption = internalclient.ManagementClusterClientOption
type ServiceClusterClient = internalclient.ServiceClusterClient
type VeleroBackupRequest = internalclient.VeleroBackupRequest
type VeleroRestoreRequest = internalclient.VeleroRestoreRequest
type VeleroRestoreResult = internalclient.VeleroRestoreResult
type VolumeSnapshotRemapRequest = internalclient.VolumeSnapshotRemapRequest
type ManifestWorkListOption = internalclient.ManifestWorkListOption
