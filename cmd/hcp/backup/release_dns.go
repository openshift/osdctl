package backup

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	ststypes "github.com/aws/aws-sdk-go-v2/service/sts/types"
	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/osdCloud"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/openshift/osdctl/pkg/utils"
	logrus "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

const (
	route53Region         = "us-east-1"
	route53MaxChangeBatch = 1000
	releaseDNSExample     = `  osdctl hcp backup release-dns --cluster-id ${CLUSTER_ID}
  osdctl hcp backup release-dns --cluster-id ${CLUSTER_ID} --profile ${AWS_PROFILE}`
)

type dnsConfig struct {
	accountID string
	zoneName  string
}

type releaseDNSFlags struct {
	clusterID  string
	awsProfile string
}

type dnsReleaseRunner struct {
	ocmConn             *ocmsdk.Connection
	resolver            ClusterResolver
	newAWSClient        func(profile, region, configFile string) (awsprovider.Client, error)
	newAWSClientInput   func(input *awsprovider.ClientInput) (awsprovider.Client, error)
	generateSessionName func(client awsprovider.Client) (string, error)
	getAWSPartition     func(client awsprovider.Client) (string, error)
	generateCredentials func(client awsprovider.Client, accountID, sessionName, partition string) (*ststypes.Credentials, error)
	getOCMEnvironment   func(*ocmsdk.Connection) string
	confirm             func(io.Reader) bool
	in                  io.Reader
	promptWriter        io.Writer
	printer             Printer
}

func newCmdReleaseDNS() *cobra.Command {
	flags := &releaseDNSFlags{}
	cmd := &cobra.Command{
		Use:               "release-dns --cluster-id <cluster-id>",
		Short:             "Release central DNS records for an HCP cluster",
		Example:           releaseDNSExample,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := logrus.New()
			logger.SetOutput(cmd.ErrOrStderr())

			ocmConn, err := utils.CreateConnection()
			if err != nil {
				return fmt.Errorf("creating OCM connection: %w", err)
			}
			defer ocmConn.Close()

			runner := newDNSReleaseRunner(ocmConn, logger, cmd.InOrStdin(), cmd.ErrOrStderr(), &defaultPrinter{w: cmd.OutOrStdout()})
			return runner.run(cmd.Context(), *flags)
		},
	}

	cmd.Flags().StringVarP(&flags.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	cmd.Flags().StringVarP(&flags.awsProfile, "profile", "p", "", "AWS profile used to assume the central DNS account role")
	_ = cmd.MarkFlagRequired("cluster-id")

	return cmd
}

func newDNSReleaseRunner(ocmConn *ocmsdk.Connection, logger *logrus.Logger, in io.Reader, promptWriter io.Writer, printer Printer) *dnsReleaseRunner {
	return &dnsReleaseRunner{
		ocmConn:             ocmConn,
		resolver:            &ocmClusterResolver{ocmConn: ocmConn, logger: logger},
		newAWSClient:        awsprovider.NewAwsClient,
		newAWSClientInput:   awsprovider.NewAwsClientWithInput,
		generateSessionName: osdCloud.GenerateRoleSessionName,
		getAWSPartition:     awsprovider.GetAwsPartition,
		generateCredentials: osdCloud.GenerateOrganizationAccountAccessCredentials,
		getOCMEnvironment:   utils.GetCurrentOCMEnv,
		confirm:             confirmDNSRelease,
		in:                  in,
		promptWriter:        promptWriter,
		printer:             printer,
	}
}

func (r *dnsReleaseRunner) run(ctx context.Context, flags releaseDNSFlags) error {
	cluster, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return fmt.Errorf("resolving cluster: %w", err)
	}

	config, err := dnsConfigForEnvironment(r.getOCMEnvironment(r.ocmConn))
	if err != nil {
		return err
	}

	baseClient, err := r.newAWSClient(flags.awsProfile, route53Region, "")
	if err != nil {
		return fmt.Errorf("creating AWS client: %w", err)
	}
	sessionName, err := r.generateSessionName(baseClient)
	if err != nil {
		return fmt.Errorf("generating AWS role session name: %w", err)
	}
	partition, err := r.getAWSPartition(baseClient)
	if err != nil {
		return fmt.Errorf("getting AWS partition: %w", err)
	}
	credentials, err := r.generateCredentials(baseClient, config.accountID, sessionName, partition)
	if err != nil {
		return fmt.Errorf("assuming central DNS account role: %w", err)
	}
	if credentials == nil || credentials.AccessKeyId == nil || credentials.SecretAccessKey == nil || credentials.SessionToken == nil {
		return fmt.Errorf("assuming central DNS account role returned incomplete credentials")
	}
	dnsClient, err := r.newAWSClientInput(&awsprovider.ClientInput{
		AccessKeyID:     *credentials.AccessKeyId,
		SecretAccessKey: *credentials.SecretAccessKey,
		SessionToken:    *credentials.SessionToken,
		Region:          route53Region,
	})
	if err != nil {
		return fmt.Errorf("creating Route53 client: %w", err)
	}

	zone, err := findHostedZone(dnsClient, config.zoneName)
	if err != nil {
		return err
	}
	targets, err := dnsRecordTargets(cluster)
	if err != nil {
		return err
	}
	records, err := findClusterRecords(dnsClient, awssdk.ToString(zone.Id), cluster.HCPClusterID, targets)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		r.printer.Printf("No A, CNAME, or TXT records for cluster %s found in hosted zone %s. Nothing to delete.\n", cluster.HCPClusterID, config.zoneName)
		return nil
	}

	r.printPreview(config, zone, cluster.HCPClusterID, records)
	if !r.confirm(r.in) {
		r.printer.Print("DNS record release cancelled.\n")
		return nil
	}

	return deleteRecords(dnsClient, awssdk.ToString(zone.Id), cluster.HCPClusterID, records)
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

func dnsRecordTargets(cluster ClusterInfo) (map[string]dnsRecordTarget, error) {
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

func (r *dnsReleaseRunner) printPreview(config dnsConfig, zone route53types.HostedZone, clusterID string, records []route53types.ResourceRecordSet) {
	r.printer.Printf("The following DNS records will be deleted for HCP cluster %s:\n", clusterID)
	r.printer.Printf("  Account: %s\n  Hosted zone: %s (%s)\n", config.accountID, config.zoneName, awssdk.ToString(zone.Id))
	for _, record := range records {
		r.printer.Printf("  %s\n", formatRecord(record))
	}
	fmt.Fprintf(r.promptWriter, "Delete these %d DNS records? (y/N): ", len(records))
}

func formatRecord(record route53types.ResourceRecordSet) string {
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

func confirmDNSRelease(in io.Reader) bool {
	var response string
	if _, err := fmt.Fscanln(in, &response); err != nil {
		return false
	}
	return strings.EqualFold(response, "y") || strings.EqualFold(response, "yes")
}
