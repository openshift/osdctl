package backup

import (
	"context"
	"io"
	"strings"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	awsmock "github.com/openshift/osdctl/pkg/provider/aws/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestDNSConfigForEnvironment(t *testing.T) {
	t.Parallel()

	tests := []struct {
		environment string
		want        dnsConfig
		wantErr     string
	}{
		{environment: "integration", want: dnsConfig{accountID: "927966746891", zoneName: "i3.devshift.org"}},
		{environment: "stage", want: dnsConfig{accountID: "927966746891", zoneName: "s3.devshift.org"}},
		{environment: "production", want: dnsConfig{accountID: "135625588012", zoneName: "p3.openshiftapps.com"}},
		{environment: "test", wantErr: "no central DNS configuration"},
	}

	for _, tt := range tests {
		t.Run(tt.environment, func(t *testing.T) {
			got, err := dnsConfigForEnvironment(tt.environment)
			if tt.wantErr != "" {
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindHostedZonePaginatesAndNormalizesName(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := awsmock.NewMockClient(ctrl)
	nextMarker := "next"
	client.EXPECT().ListHostedZones(gomock.Cond(func(input *route53.ListHostedZonesInput) bool {
		return input.Marker == nil
	})).Return(&route53.ListHostedZonesOutput{
		IsTruncated: true,
		NextMarker:  &nextMarker,
		HostedZones: []route53types.HostedZone{{Id: awssdk.String("/hostedzone/other"), Name: awssdk.String("other.example.com.")}},
	}, nil)
	client.EXPECT().ListHostedZones(gomock.Cond(func(input *route53.ListHostedZonesInput) bool {
		return awssdk.ToString(input.Marker) == nextMarker
	})).Return(&route53.ListHostedZonesOutput{
		HostedZones: []route53types.HostedZone{{Id: awssdk.String("/hostedzone/target"), Name: awssdk.String("target.example.com.")}},
	}, nil)

	zone, err := findHostedZone(client, "target.example.com")
	require.NoError(t, err)
	assert.Equal(t, "/hostedzone/target", awssdk.ToString(zone.Id))
}

func TestFindClusterRecordsFiltersSortsAndPaginates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := awsmock.NewMockClient(ctrl)
	nextName := "next.cluster.example.com."
	nextIdentifier := "weighted"
	client.EXPECT().ListResourceRecordSets(gomock.Cond(func(input *route53.ListResourceRecordSetsInput) bool {
		return awssdk.ToString(input.HostedZoneId) == "/hostedzone/target" && input.StartRecordName == nil
	})).Return(&route53.ListResourceRecordSetsOutput{
		IsTruncated:          true,
		NextRecordName:       &nextName,
		NextRecordType:       route53types.RRTypeA,
		NextRecordIdentifier: &nextIdentifier,
		ResourceRecordSets: []route53types.ResourceRecordSet{
			{
				Name: awssdk.String("api-external-dns.cluster.example.com."),
				Type: route53types.RRTypeTxt,
				ResourceRecords: []route53types.ResourceRecord{{
					Value: awssdk.String("\"external-dns/resource=route/ocm-staging-cluster/kube-apiserver\""),
				}},
			},
			{Name: awssdk.String("api.cluster.example.com."), Type: route53types.RRTypeA},
			{Name: awssdk.String("api.cluster.example.com."), Type: route53types.RRTypeTxt, ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String("\"cluster\"")}}},
			{Name: awssdk.String("ignored-cluster.example.com."), Type: route53types.RRTypeA},
		},
	}, nil)
	client.EXPECT().ListResourceRecordSets(gomock.Cond(func(input *route53.ListResourceRecordSetsInput) bool {
		return awssdk.ToString(input.StartRecordName) == nextName &&
			input.StartRecordType == route53types.RRTypeA &&
			awssdk.ToString(input.StartRecordIdentifier) == nextIdentifier
	})).Return(&route53.ListResourceRecordSetsOutput{ResourceRecordSets: []route53types.ResourceRecordSet{
		{Name: awssdk.String("oauth.cluster.example.com."), Type: route53types.RRTypeCname},
		{Name: awssdk.String("a-api-external-dns.cluster.example.com."), Type: route53types.RRTypeTxt, ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String("\"external-dns/resource=route/other/unrelated\"")}}},
		{Name: awssdk.String("cname-api-external-dns.cluster.example.com."), Type: route53types.RRTypeA},
	}}, nil)

	targets, err := dnsRecordTargets(ClusterInfo{HCPClusterID: "cluster", DomainPrefix: "cluster", BaseDomain: "example.com"})
	require.NoError(t, err)
	records, err := findClusterRecords(client, "/hostedzone/target", "cluster", targets)
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"api-external-dns.cluster.example.com.", "api.cluster.example.com.", "oauth.cluster.example.com."},
		[]string{awssdk.ToString(records[0].Name), awssdk.ToString(records[1].Name), awssdk.ToString(records[2].Name)},
	)
}

func TestDNSRecordTargets(t *testing.T) {
	t.Parallel()

	targets, err := dnsRecordTargets(ClusterInfo{HCPClusterID: "cluster", DomainPrefix: "prefix", BaseDomain: "base.example.com"})
	require.NoError(t, err)
	assert.Len(t, targets, 8)
	for _, name := range []string{
		"api.prefix.base.example.com",
		"oauth.prefix.base.example.com",
		"api-external-dns.prefix.base.example.com",
		"oauth-external-dns.prefix.base.example.com",
		"a-api-external-dns.prefix.base.example.com",
		"a-oauth-external-dns.prefix.base.example.com",
		"cname-api-external-dns.prefix.base.example.com",
		"cname-oauth-external-dns.prefix.base.example.com",
	} {
		assert.Contains(t, targets, name)
	}

	_, err = dnsRecordTargets(ClusterInfo{HCPClusterID: "cluster", BaseDomain: "base.example.com"})
	assert.ErrorContains(t, err, "no domain prefix")
	_, err = dnsRecordTargets(ClusterInfo{HCPClusterID: "cluster", DomainPrefix: "prefix"})
	assert.ErrorContains(t, err, "no DNS base domain")
}

func TestDeleteRecordsBatchesAndPreservesRecordSets(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := awsmock.NewMockClient(ctrl)
	records := make([]route53types.ResourceRecordSet, route53MaxChangeBatch+1)
	for i := range records {
		records[i] = route53types.ResourceRecordSet{
			Name:            awssdk.String("cluster.example.com."),
			Type:            route53types.RRTypeA,
			TTL:             awssdk.Int64(60),
			ResourceRecords: []route53types.ResourceRecord{{Value: awssdk.String("192.0.2.1")}},
		}
	}
	client.EXPECT().ChangeResourceRecordSets(gomock.Cond(func(input *route53.ChangeResourceRecordSetsInput) bool {
		return awssdk.ToString(input.HostedZoneId) == "/hostedzone/target" &&
			len(input.ChangeBatch.Changes) == route53MaxChangeBatch &&
			input.ChangeBatch.Changes[0].Action == route53types.ChangeActionDelete &&
			input.ChangeBatch.Changes[0].ResourceRecordSet == &records[0]
	})).Return(&route53.ChangeResourceRecordSetsOutput{}, nil)
	client.EXPECT().ChangeResourceRecordSets(gomock.Cond(func(input *route53.ChangeResourceRecordSetsInput) bool {
		return len(input.ChangeBatch.Changes) == 1 && input.ChangeBatch.Changes[0].ResourceRecordSet == &records[route53MaxChangeBatch]
	})).Return(&route53.ChangeResourceRecordSetsOutput{}, nil)

	require.NoError(t, deleteRecords(client, "/hostedzone/target", "cluster", records))
}

func TestDNSReleaseRunnerDeletesConfirmedRecords(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	baseClient := awsmock.NewMockClient(ctrl)
	dnsClient := awsmock.NewMockClient(ctrl)
	dnsClient.EXPECT().ListHostedZones(gomock.Any()).Return(&route53.ListHostedZonesOutput{HostedZones: []route53types.HostedZone{{
		Id:   awssdk.String("/hostedzone/target"),
		Name: awssdk.String("s3.devshift.org."),
	}}}, nil)
	record := route53types.ResourceRecordSet{
		Name:          awssdk.String("api.cluster.s3.devshift.org."),
		Type:          route53types.RRTypeCname,
		TTL:           awssdk.Int64(300),
		SetIdentifier: awssdk.String("blue"),
		ResourceRecords: []route53types.ResourceRecord{{
			Value: awssdk.String("target.example.com."),
		}},
	}
	dnsClient.EXPECT().ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{ResourceRecordSets: []route53types.ResourceRecordSet{record}}, nil)
	dnsClient.EXPECT().ChangeResourceRecordSets(gomock.Cond(func(input *route53.ChangeResourceRecordSetsInput) bool {
		change := input.ChangeBatch.Changes[0]
		return len(input.ChangeBatch.Changes) == 1 &&
			change.Action == route53types.ChangeActionDelete &&
			change.ResourceRecordSet != nil &&
			awssdk.ToString(change.ResourceRecordSet.Name) == awssdk.ToString(record.Name) &&
			change.ResourceRecordSet.Type == record.Type &&
			awssdk.ToString(change.ResourceRecordSet.SetIdentifier) == awssdk.ToString(record.SetIdentifier) &&
			awssdk.ToInt64(change.ResourceRecordSet.TTL) == awssdk.ToInt64(record.TTL) &&
			awssdk.ToString(change.ResourceRecordSet.ResourceRecords[0].Value) == awssdk.ToString(record.ResourceRecords[0].Value)
	})).Return(&route53.ChangeResourceRecordSetsOutput{}, nil)

	var output, promptOutput strings.Builder
	runner := &dnsReleaseRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "cluster", DomainPrefix: "cluster", BaseDomain: "s3.devshift.org"}},
		newAWSClient: func(profile, region, configFile string) (awsprovider.Client, error) {
			assert.Equal(t, "profile", profile)
			assert.Equal(t, route53Region, region)
			return baseClient, nil
		},
		generateSessionName: func(client awsprovider.Client) (string, error) {
			assert.Same(t, baseClient, client)
			return "session", nil
		},
		getAWSPartition: func(client awsprovider.Client) (string, error) {
			assert.Same(t, baseClient, client)
			return "aws", nil
		},
		generateCredentials: func(client awsprovider.Client, accountID, sessionName, partition string) (*ststypes.Credentials, error) {
			assert.Same(t, baseClient, client)
			assert.Equal(t, "927966746891", accountID)
			assert.Equal(t, "session", sessionName)
			assert.Equal(t, "aws", partition)
			return &ststypes.Credentials{AccessKeyId: awssdk.String("key"), SecretAccessKey: awssdk.String("secret"), SessionToken: awssdk.String("token")}, nil
		},
		newAWSClientInput: func(input *awsprovider.ClientInput) (awsprovider.Client, error) {
			assert.Equal(t, &awsprovider.ClientInput{AccessKeyID: "key", SecretAccessKey: "secret", SessionToken: "token", Region: route53Region}, input)
			return dnsClient, nil
		},
		getOCMEnvironment: func(_ *ocmsdk.Connection) string { return "stage" },
		confirm:           confirmDNSRelease,
		in:                strings.NewReader("yes\n"),
		promptWriter:      &promptOutput,
		printer:           &defaultPrinter{w: &output},
	}

	require.NoError(t, runner.run(context.Background(), releaseDNSFlags{clusterID: "cluster", awsProfile: "profile"}))
	assert.Contains(t, output.String(), "api.cluster.s3.devshift.org. CNAME set=blue ttl=300 values=target.example.com.")
	assert.NotContains(t, output.String(), "Delete these")
	assert.Contains(t, promptOutput.String(), "Delete these 1 DNS records? (y/N):")
}

func TestDNSReleaseRunnerDoesNotDeleteWithoutRecordsOrConfirmation(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		records []route53types.ResourceRecordSet
		input   string
		want    string
	}{
		{name: "no records", want: "Nothing to delete"},
		{name: "declined", records: []route53types.ResourceRecordSet{{Name: awssdk.String("api.cluster.i3.devshift.org."), Type: route53types.RRTypeA}}, input: "no\n", want: "cancelled"},
		{name: "eof", records: []route53types.ResourceRecordSet{{Name: awssdk.String("api.cluster.i3.devshift.org."), Type: route53types.RRTypeA}}, want: "cancelled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			baseClient := awsmock.NewMockClient(ctrl)
			dnsClient := awsmock.NewMockClient(ctrl)
			dnsClient.EXPECT().ListHostedZones(gomock.Any()).Return(&route53.ListHostedZonesOutput{HostedZones: []route53types.HostedZone{{Id: awssdk.String("zone"), Name: awssdk.String("i3.devshift.org.")}}}, nil)
			dnsClient.EXPECT().ListResourceRecordSets(gomock.Any()).Return(&route53.ListResourceRecordSetsOutput{ResourceRecordSets: tt.records}, nil)

			var output strings.Builder
			runner := dnsTestRunner(baseClient, dnsClient, &output, strings.NewReader(tt.input))
			require.NoError(t, runner.run(context.Background(), releaseDNSFlags{clusterID: "cluster"}))
			assert.Contains(t, output.String(), tt.want)
		})
	}
}

func TestDNSReleaseRunnerRejectsIncompleteCredentials(t *testing.T) {
	t.Parallel()

	baseClient := awsmock.NewMockClient(gomock.NewController(t))
	runner := dnsTestRunner(baseClient, nil, &strings.Builder{}, strings.NewReader("yes\n"))
	runner.generateCredentials = func(awsprovider.Client, string, string, string) (*ststypes.Credentials, error) {
		return nil, nil
	}
	err := runner.run(context.Background(), releaseDNSFlags{clusterID: "cluster"})
	assert.ErrorContains(t, err, "incomplete credentials")
}

func TestDNSReleaseFlags(t *testing.T) {
	t.Parallel()

	cmd := newCmdReleaseDNS()
	assert.NotNil(t, cmd.Flags().Lookup("cluster-id"))
	assert.NotNil(t, cmd.Flags().Lookup("profile"))
}

func dnsTestRunner(baseClient, dnsClient awsprovider.Client, output *strings.Builder, input *strings.Reader) *dnsReleaseRunner {
	return &dnsReleaseRunner{
		resolver: &staticClusterResolver{clusterInfo: ClusterInfo{HCPClusterID: "cluster", DomainPrefix: "cluster", BaseDomain: "i3.devshift.org"}},
		newAWSClient: func(string, string, string) (awsprovider.Client, error) {
			return baseClient, nil
		},
		generateSessionName: func(awsprovider.Client) (string, error) { return "session", nil },
		getAWSPartition:     func(awsprovider.Client) (string, error) { return "aws", nil },
		generateCredentials: func(awsprovider.Client, string, string, string) (*ststypes.Credentials, error) {
			return &ststypes.Credentials{AccessKeyId: awssdk.String("key"), SecretAccessKey: awssdk.String("secret"), SessionToken: awssdk.String("token")}, nil
		},
		newAWSClientInput: func(*awsprovider.ClientInput) (awsprovider.Client, error) { return dnsClient, nil },
		getOCMEnvironment: func(*ocmsdk.Connection) string { return "integration" },
		confirm:           confirmDNSRelease,
		in:                input,
		promptWriter:      io.Discard,
		printer:           &defaultPrinter{w: output},
	}
}
