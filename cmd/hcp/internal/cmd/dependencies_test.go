package cmd

import (
	"context"
	"fmt"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

type awsAccountClientCall struct {
	profile   string
	region    string
	accountID string
}

type testAWSAccountClientProvider struct {
	client awsprovider.Client
	err    error
	calls  []awsAccountClientCall
}

func (p *testAWSAccountClientProvider) AccountClient(profile, region, accountID string) (awsprovider.Client, error) {
	p.calls = append(p.calls, awsAccountClientCall{profile: profile, region: region, accountID: accountID})
	return p.client, p.err
}

type testDiscoveryOCMClient struct {
	cluster ClusterInfo
	config  *cmv1.AWSBackupConfig
	err     error
}

func (c *testDiscoveryOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.cluster, c.err
}

func (c *testDiscoveryOCMClient) BackupConfig(_ context.Context, _ string, _ string) (*cmv1.AWSBackupConfig, error) {
	return c.config, c.err
}

type testDNSOCMClient struct {
	cluster     ClusterInfo
	environment string
	err         error
}

func (c *testDNSOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.cluster, c.err
}

func (c *testDNSOCMClient) Environment() string { return c.environment }

type testTrustPolicyOCMClient struct {
	cluster  ClusterInfo
	clusters map[string]*cmv1.Cluster
	configs  map[string]*cmv1.AWSBackupConfig
	err      error
}

func (c *testTrustPolicyOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.cluster, c.err
}

func (c *testTrustPolicyOCMClient) Cluster(identifier string) (*cmv1.Cluster, error) {
	cluster, ok := c.clusters[identifier]
	if !ok {
		return nil, fmt.Errorf("unexpected identifier %s", identifier)
	}
	return cluster, nil
}

func (c *testTrustPolicyOCMClient) BackupConfig(_ context.Context, _ string, managementCluster string) (*cmv1.AWSBackupConfig, error) {
	config, ok := c.configs[managementCluster]
	if !ok {
		return nil, fmt.Errorf("unexpected management cluster %s", managementCluster)
	}
	return config, nil
}

type testManifestWorkOCMClient struct {
	cluster        ClusterInfo
	serviceCluster ServiceClusterInfo
	client         ServiceClusterClient
	resolved       []string
	calls          []serviceClusterCall
}

func (c *testManifestWorkOCMClient) Resolve(_ context.Context, _ string) (ClusterInfo, error) {
	return c.cluster, nil
}

func (c *testManifestWorkOCMClient) ResolveServiceCluster(_ context.Context, clusterID string) (ServiceClusterInfo, error) {
	c.resolved = append(c.resolved, clusterID)
	return c.serviceCluster, nil
}

func (c *testManifestWorkOCMClient) ServiceCluster(clusterID, reason string) (ServiceClusterClient, error) {
	c.calls = append(c.calls, serviceClusterCall{clusterID: clusterID, reason: reason})
	return c.client, nil
}
