package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	internalclient "github.com/openshift/osdctl/cmd/hcp/internal/client"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

const (
	route53Region         = "us-east-1"
	route53MaxChangeBatch = 1000
	releaseDNSExample     = `  osdctl hcp restore release-dns --cluster-id ${CLUSTER_ID}
   osdctl hcp restore release-dns --cluster-id ${CLUSTER_ID} --profile ${AWS_PROFILE}`
)

type dnsConfig struct {
	accountID string
	zoneName  string
}

type ReleaseDNSRequest struct {
	ClusterID  string
	AWSProfile string
}

// DNSReleaser prepares and applies central DNS record removal for an HCP cluster.
type DNSReleaser interface {
	PrepareDNSRelease(ctx context.Context, request ReleaseDNSRequest) (DNSReleasePlan, error)
	ReleaseDNS(ctx context.Context, plan DNSReleasePlan) error
}

// DNSReleasePlan describes the records selected for deletion.
type DNSReleasePlan struct {
	ClusterID string
	AccountID string
	ZoneName  string
	ZoneID    string
	Records   []route53types.ResourceRecordSet

	client awsprovider.Client
}

// DefaultDNSReleaser is the production DNS release implementation.
type DefaultDNSReleaser struct {
	ocmClient dnsOCMClient
	awsClient internalclient.AWSAccountClientProvider
}

type dnsOCMClient interface {
	internalclient.ClusterResolver
	Environment() string
}

func NewDefaultDNSReleaser(ocmClient dnsOCMClient) *DefaultDNSReleaser {
	return &DefaultDNSReleaser{
		ocmClient: ocmClient,
		awsClient: internalclient.NewAWSAccountClientProvider(),
	}
}

func (r *DefaultDNSReleaser) PrepareDNSRelease(ctx context.Context, request ReleaseDNSRequest) (DNSReleasePlan, error) {
	cluster, err := r.ocmClient.Resolve(ctx, request.ClusterID)
	if err != nil {
		return DNSReleasePlan{}, fmt.Errorf("resolving cluster: %w", err)
	}

	config, err := dnsConfigForEnvironment(r.ocmClient.Environment())
	if err != nil {
		return DNSReleasePlan{}, err
	}

	dnsClient, err := r.awsClient.AccountClient(request.AWSProfile, route53Region, config.accountID)
	if err != nil {
		return DNSReleasePlan{}, fmt.Errorf("creating Route53 client: %w", err)
	}

	zone, err := findHostedZone(dnsClient, config.zoneName)
	if err != nil {
		return DNSReleasePlan{}, err
	}
	targets, err := dnsRecordTargets(cluster)
	if err != nil {
		return DNSReleasePlan{}, err
	}
	records, err := findClusterRecords(dnsClient, awssdk.ToString(zone.Id), cluster.HCPClusterID, targets)
	if err != nil {
		return DNSReleasePlan{}, err
	}
	return DNSReleasePlan{ClusterID: cluster.HCPClusterID, AccountID: config.accountID, ZoneName: config.zoneName, ZoneID: awssdk.ToString(zone.Id), Records: records, client: dnsClient}, nil
}

func (r *DefaultDNSReleaser) ReleaseDNS(ctx context.Context, plan DNSReleasePlan) error {
	return deleteRecords(plan.client, plan.ZoneID, plan.ClusterID, plan.Records)
}

func dnsConfigForEnvironment(environment string) (dnsConfig, error) {
	switch environment {
	case "integration":
		return dnsConfig{accountID: "927966746891", zoneName: "i3.devshift.org"}, nil
	case "stage":
		return dnsConfig{accountID: "927966746891", zoneName: "s3.devshift.org"}, nil
	case "production":
		return dnsConfig{accountID: "135625588012", zoneName: "p3.openshiftapps.com"}, nil
	default:
		return dnsConfig{}, fmt.Errorf("no central DNS configuration for OCM environment %q", environment)
	}
}

func findHostedZone(client awsprovider.Client, zoneName string) (route53types.HostedZone, error) {
	var matching *route53types.HostedZone
	var marker *string
	for {
		output, err := client.ListHostedZones(&route53.ListHostedZonesInput{Marker: marker})
		if err != nil {
			return route53types.HostedZone{}, fmt.Errorf("listing hosted zones: %w", err)
		}
		if output == nil {
			return route53types.HostedZone{}, fmt.Errorf("listing hosted zones returned no result")
		}
		for _, zone := range output.HostedZones {
			if normalizeDNSName(awssdk.ToString(zone.Name)) != normalizeDNSName(zoneName) {
				continue
			}
			if matching != nil {
				return route53types.HostedZone{}, fmt.Errorf("multiple hosted zones named %s found", zoneName)
			}
			zone := zone
			matching = &zone
		}
		if !output.IsTruncated {
			break
		}
		if output.NextMarker == nil || *output.NextMarker == "" {
			return route53types.HostedZone{}, fmt.Errorf("listing hosted zones returned no next marker")
		}
		marker = output.NextMarker
	}
	if matching == nil {
		return route53types.HostedZone{}, fmt.Errorf("hosted zone %s not found", zoneName)
	}
	if matching.Id == nil || *matching.Id == "" {
		return route53types.HostedZone{}, fmt.Errorf("hosted zone %s has no ID", zoneName)
	}
	return *matching, nil
}

type dnsRecordTarget struct {
	types             map[route53types.RRType]struct{}
	requiresClusterID bool
}

func dnsRecordTargets(cluster internalclient.ClusterInfo) (map[string]dnsRecordTarget, error) {
	domainPrefix := normalizeDNSName(cluster.DomainPrefix)
	baseDomain := normalizeDNSName(cluster.BaseDomain)
	if domainPrefix == "" {
		return nil, fmt.Errorf("cluster %s has no domain prefix", cluster.HCPClusterID)
	}
	if baseDomain == "" {
		return nil, fmt.Errorf("cluster %s has no DNS base domain", cluster.HCPClusterID)
	}

	baseName := fmt.Sprintf("%s.%s", domainPrefix, baseDomain)
	targets := make(map[string]dnsRecordTarget, 8)
	endpointTypes := map[route53types.RRType]struct{}{
		route53types.RRTypeA:     {},
		route53types.RRTypeCname: {},
	}
	txtTypes := map[route53types.RRType]struct{}{route53types.RRTypeTxt: {}}
	for _, endpoint := range []string{"api", "oauth"} {
		targets[fmt.Sprintf("%s.%s", endpoint, baseName)] = dnsRecordTarget{types: endpointTypes}
		for _, prefix := range []string{"", "a-", "cname-"} {
			targets[fmt.Sprintf("%s%s-external-dns.%s", prefix, endpoint, baseName)] = dnsRecordTarget{
				types:             txtTypes,
				requiresClusterID: true,
			}
		}
	}
	return targets, nil
}

func findClusterRecords(client awsprovider.Client, hostedZoneID, clusterID string, targets map[string]dnsRecordTarget) ([]route53types.ResourceRecordSet, error) {
	var records []route53types.ResourceRecordSet
	input := &route53.ListResourceRecordSetsInput{HostedZoneId: awssdk.String(hostedZoneID)}
	for {
		output, err := client.ListResourceRecordSets(input)
		if err != nil {
			return nil, fmt.Errorf("listing record sets in hosted zone %s: %w", hostedZoneID, err)
		}
		if output == nil {
			return nil, fmt.Errorf("listing record sets in hosted zone %s returned no result", hostedZoneID)
		}
		for _, record := range output.ResourceRecordSets {
			if isClusterRecord(record, clusterID, targets) {
				records = append(records, record)
			}
		}
		if !output.IsTruncated {
			break
		}
		if output.NextRecordName == nil || output.NextRecordType == "" {
			return nil, fmt.Errorf("listing record sets in hosted zone %s returned incomplete continuation data", hostedZoneID)
		}
		input = &route53.ListResourceRecordSetsInput{
			HostedZoneId:          awssdk.String(hostedZoneID),
			StartRecordName:       output.NextRecordName,
			StartRecordType:       output.NextRecordType,
			StartRecordIdentifier: output.NextRecordIdentifier,
		}
	}

	sort.Slice(records, func(i, j int) bool {
		left, right := records[i], records[j]
		if awssdk.ToString(left.Name) != awssdk.ToString(right.Name) {
			return awssdk.ToString(left.Name) < awssdk.ToString(right.Name)
		}
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		return awssdk.ToString(left.SetIdentifier) < awssdk.ToString(right.SetIdentifier)
	})
	return records, nil
}

func isClusterRecord(record route53types.ResourceRecordSet, clusterID string, targets map[string]dnsRecordTarget) bool {
	target, ok := targets[normalizeDNSName(awssdk.ToString(record.Name))]
	if !ok {
		return false
	}
	if _, ok := target.types[record.Type]; !ok {
		return false
	}
	if !target.requiresClusterID {
		return true
	}
	for _, resourceRecord := range record.ResourceRecords {
		if strings.Contains(awssdk.ToString(resourceRecord.Value), clusterID) {
			return true
		}
	}
	return false
}

func deleteRecords(client awsprovider.Client, hostedZoneID, clusterID string, records []route53types.ResourceRecordSet) error {
	for start := 0; start < len(records); start += route53MaxChangeBatch {
		end := min(start+route53MaxChangeBatch, len(records))
		changes := make([]route53types.Change, 0, end-start)
		for i := start; i < end; i++ {
			changes = append(changes, route53types.Change{
				Action:            route53types.ChangeActionDelete,
				ResourceRecordSet: &records[i],
			})
		}
		if _, err := client.ChangeResourceRecordSets(&route53.ChangeResourceRecordSetsInput{
			HostedZoneId: awssdk.String(hostedZoneID),
			ChangeBatch: &route53types.ChangeBatch{
				Comment: awssdk.String(fmt.Sprintf("Delete external-dns and HCP records for %s", clusterID)),
				Changes: changes,
			},
		}); err != nil {
			return fmt.Errorf("deleting DNS records batch %d: %w", start/route53MaxChangeBatch+1, err)
		}
	}
	return nil
}

// FormatDNSRecord renders one DNS record for a release preview.
func FormatDNSRecord(record route53types.ResourceRecordSet) string {
	details := []string{fmt.Sprintf("%s %s", awssdk.ToString(record.Name), record.Type)}
	if record.SetIdentifier != nil {
		details = append(details, "set="+*record.SetIdentifier)
	}
	if record.AliasTarget != nil {
		details = append(details, "alias="+awssdk.ToString(record.AliasTarget.DNSName))
	} else {
		details = append(details, fmt.Sprintf("ttl=%d", awssdk.ToInt64(record.TTL)))
		values := make([]string, 0, len(record.ResourceRecords))
		for _, value := range record.ResourceRecords {
			values = append(values, awssdk.ToString(value.Value))
		}
		if len(values) > 0 {
			details = append(details, "values="+strings.Join(values, ", "))
		}
	}
	return strings.Join(details, " ")
}

func normalizeDNSName(name string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
}
