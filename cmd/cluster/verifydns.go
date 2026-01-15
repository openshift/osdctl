package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/cluster/internal/dns"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// verifyDNSOptions defines the struct for running the DNS verify command
type verifyDNSOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
	verbose   bool
	output    string
	genericclioptions.IOStreams
	analyzer dns.Analyzer
	verifier dns.Verifier
}

// NewCmdVerifyDNS implements the verify-dns command
func NewCmdVerifyDNS(streams genericclioptions.IOStreams) *cobra.Command {
	ops := &verifyDNSOptions{
		IOStreams: streams,
		verifier:  dns.NewDefaultVerifier(),
		analyzer: dns.NewDefaultAnalyzer(dns.WithRecommender{
			Recommender: &recommender{},
		}),
	}

	verifyDNSCmd := &cobra.Command{
		Use:               "verify-dns --cluster-id <cluster-id>",
		Short:             "Verify DNS resolution for HCP cluster public endpoints",
		Long:              verifyDNSLongDescription,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run(cmd.Context()))
		},
	}

	verifyDNSCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID (internal or external)")
	verifyDNSCmd.Flags().BoolVarP(&ops.verbose, "verbose", "v", false, "Verbose output")
	verifyDNSCmd.Flags().StringVarP(&ops.output, "output", "o", "table", "Output format: 'table' or 'json'")

	if err := verifyDNSCmd.MarkFlagRequired("cluster-id"); err != nil {
		panic(fmt.Sprintf("failed to mark cluster-id flag as required: %v", err))
	}

	return verifyDNSCmd
}

//go:embed verify_dns_long_description.txt
var verifyDNSLongDescription string

func (v *verifyDNSOptions) complete(cmd *cobra.Command) error {
	if v.clusterID == "" {
		return fmt.Errorf("cluster-id is required")
	}
	return nil
}

func (v *verifyDNSOptions) run(ctx context.Context) error {
	if v.verbose {
		fmt.Fprintf(v.Out, "Retrieving cluster information for: %s\n", v.clusterID)
	}

	cluster, err := v.getCluster()
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	if !cluster.Hypershift().Enabled() {
		return fmt.Errorf("cluster %s is not an HCP cluster. This command only supports HCP clusters", v.clusterID)
	}

	if v.verbose {
		fmt.Fprintf(v.Out, "Cluster %s is an HCP cluster\n", cluster.Name())
	}

	if cluster.State() != cmv1.ClusterStateReady {
		return fmt.Errorf("cluster %s is not in Ready state. Current state: %s", cluster.Name(), cluster.State())
	}

	if v.verbose {
		fmt.Fprintf(v.Out, "Performing DNS resolution test\n")
	}

	testcases := v.buildTestCases(cluster)

	var wg sync.WaitGroup
	resultCh := make(chan dns.VerifyResult, len(testcases))
	for _, tc := range testcases {
		wg.Add(1)
		go func(c chan<- dns.VerifyResult, t dnstestCase) {
			defer wg.Done()
			if t.skip {
				c <- dns.VerifyResult{
					Name:       t.name,
					Type:       t.recordType,
					Status:     "SKIP",
					SkipReason: "Skipped due to cluster creation date before March 10, 2025",
				}
				return
			}
			if t.recordType == dns.RecordTypeCNAME {
				c <- v.verifier.VerifyCNAMERecord(ctx, t.name, dns.WithExpectedTarget(t.expectedTarget))
			}
			if t.recordType == dns.RecordTypeA {
				c <- v.verifier.VerifyARecord(ctx, t.name)
			}
		}(resultCh, tc)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	results := make([]dns.VerifyResult, 0, len(testcases))
	for res := range resultCh {
		results = append(results, res)
	}

	report := v.analyzer.Analyze(cluster, results)

	// Output based on format
	switch v.output {
	case "json":
		jsonOutput, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling report: %w", err)
		}
		fmt.Fprintln(v.Out, string(jsonOutput))
	case "table":
		v.renderTable(report)
	default:
		return fmt.Errorf("invalid output format: %s (must be 'table' or 'json')", v.output)
	}

	return nil
}

func (v *verifyDNSOptions) getCluster() (*cmv1.Cluster, error) {
	if v.cluster != nil {
		return v.cluster, nil
	}

	conn, err := utils.CreateConnection()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	v.cluster, err = utils.GetCluster(conn, v.clusterID)
	return v.cluster, err
}

func (v *verifyDNSOptions) buildTestCases(cluster *cmv1.Cluster) map[string]dnstestCase {
	tests := make(map[string]dnstestCase)

	tests["console"] = dnstestCase{
		name:       cluster.Console().URL(),
		recordType: "A",
		description: "Test Console A record: console-openshift-console.apps.rosa.<cluster-name>.<base-domain>." +
			"This verifies the presence of the A record for the wildcard domain " +
			"*.apps.rosa.<cluster-name>.<base-domain>",
	}

	name := cluster.Name()
	id := cluster.ID()
	domain := cluster.DNS().BaseDomain()

	clusterSubDomain := fmt.Sprintf("%s.%s", name, domain)
	actualChallengeRecord := fmt.Sprintf("_acme-challenge.%s.%s", name, domain)

	defaultIngressFQDN := fmt.Sprintf("apps.rosa.%s.%s", name, domain)
	tests["default_ingress"] = dnstestCase{
		name:           defaultIngressFQDN,
		recordType:     dns.RecordTypeCNAME,
		description:    "Test CNAME: apps.rosa.<cluster-name>.<base-domain> -> <cluster-name>.<base-domain>",
		expectedTarget: clusterSubDomain,
	}

	// 3. Test CNAME: _acme-challenge.apps.rosa.<cluster-name>.<base-domain> -> _acme-challenge.<cluster-name>.<base-domain>
	defaultIngressChallengePointer := fmt.Sprintf("_acme-challenge.apps.rosa.%s.%s", name, domain)
	tests["default_ingress_challenge"] = dnstestCase{
		name:           defaultIngressChallengePointer,
		recordType:     dns.RecordTypeCNAME,
		description:    "Test CNAME: _acme-challenge.apps.rosa.<cluster-name>.<base-domain> -> _acme-challenge.<cluster-name>.<base-domain>",
		expectedTarget: actualChallengeRecord,
	}

	uniqueFQDN := fmt.Sprintf("%s.rosa.%s.%s", id, name, domain)
	uniqueTest := dnstestCase{
		name:           uniqueFQDN,
		recordType:     dns.RecordTypeCNAME,
		description:    "Test CNAME: <cluster-id>.rosa.<cluster-name>.<base-domain> -> <cluster-name>.<base-domain>",
		expectedTarget: clusterSubDomain,
	}

	shouldSkipUnique := v.shouldSkipUniqueFQDN(cluster)
	if shouldSkipUnique {
		uniqueTest.skip = true
	}
	tests["unique"] = uniqueTest

	uniqueChallengePointer := fmt.Sprintf("_acme-challenge.%s.rosa.%s.%s", id, name, domain)
	uniqueChallengeTest := dnstestCase{
		name:           uniqueChallengePointer,
		recordType:     dns.RecordTypeCNAME,
		description:    "Test CNAME: _acme-challenge.<cluster-id>.rosa.<cluster-name>.<base-domain> -> _acme-challenge.<cluster-name>.<base-domain>",
		expectedTarget: actualChallengeRecord,
	}
	if shouldSkipUnique {
		uniqueChallengeTest.skip = true
	}
	tests["unique_challenge"] = uniqueChallengeTest

	apiFQDN := fmt.Sprintf("api.%s.%s", name, domain)
	apiFQDNTest := dnstestCase{
		name: apiFQDN,
	}

	oauthFQDN := fmt.Sprintf("oauth.%s.%s", name, domain)
	oauthFQDNTest := dnstestCase{
		name: oauthFQDN,
	}

	if cluster.AWS().PrivateLink() {
		apiFQDNTest.recordType = dns.RecordTypeCNAME
		apiFQDNTest.description = "Test CNAME: api.<cluster-name>.<base-domain>"
		oauthFQDNTest.recordType = dns.RecordTypeCNAME
		oauthFQDNTest.description = "Test CNAME: oauth.<cluster-name>.<base-domain>"
	} else {
		oauthFQDNTest.recordType = dns.RecordTypeA
		oauthFQDNTest.description = "Test A record: oauth.<cluster-name>.<base-domain>"
		apiFQDNTest.recordType = dns.RecordTypeA
		apiFQDNTest.description = "Test A record: api.<cluster-name>.<base-domain>"
	}
	tests["api"] = apiFQDNTest
	tests["oauth"] = oauthFQDNTest

	return tests
}

func (v *verifyDNSOptions) shouldSkipUniqueFQDN(cluster *cmv1.Cluster) bool {
	cutoffDate := time.Date(2025, time.March, 10, 0, 0, 0, 0, time.UTC)
	creationTime := cluster.CreationTimestamp()

	// Skip if cluster created before March 10, 2025
	return creationTime.Before(cutoffDate)
}

type dnstestCase struct {
	name           string
	recordType     dns.RecordType
	description    string
	expectedTarget string // For CNAME records
	skip           bool
}

type recommender struct{}

func (r *recommender) MakeRecommendations(results []dns.VerifyResult, opts ...dns.MakeRecommendationsOption) []string {
	var cfg dns.MakeRecommendationsConfig
	cfg.Option(opts...)

	var recommendations []string
	for _, res := range results {
		if res.Status != dns.VerifyResultStatusFail {
			continue
		}

		if strings.HasPrefix(res.Name, "console") {
			recommendations = append(recommendations, strings.Join([]string{
				"If the console FQDN is not resolving then there is likely an issue with",
				"CIO on the HCP cluster. Check if the A record <*.apps.rosa.<cluster-name>.<base-domain>",
				"is defined in the Route 53 Public Hosted Zone in the customer AWS account.",
				"If not check the health of CIO in the customer cluster.",
			}, " "))
		} else if strings.HasPrefix(res.Name, "apps.rosa") || strings.HasPrefix(res.Name, "_acme-challenge.apps.rosa") {
			recommendations = append(recommendations, strings.Join([]string{
				"If the default ingress FQDNs are not resolving then there is likely an issue with",
				"the Route 53 configuration in the customer AWS account. These records are created",
				"once during provisioning by OCM and are not reconciled afterwards. Check the Route 53",
				"public hosted zone in the customer AWS account to ensure these records exist.",
				"The CNAME record <_acme-challenge.apps.rosa.<cluster-name>.<base-domain> in particular",
				"must exist for ingress certificate issuance and renewal to succeed.",
			}, " "))
		} else if strings.HasPrefix(res.Name, cfg.Cluster.ID()) || strings.HasPrefix(res.Name, "_acme-challenge."+cfg.Cluster.ID()) {
			recommendations = append(recommendations, strings.Join([]string{
				"If the unique FQDNs are not resolving then there is likely an issue with",
				"the Route 53 configuration in the customer AWS account. These records are created",
				"once during provisioning by OCM and are not reconciled afterwards. Check the Route 53",
				"public hosted zone in the customer AWS account to ensure these records exist.",
				"The both CNAME records must exist for ingress certificate issuance and renewal to succeed.",
			}, " "))
		} else if strings.HasPrefix(res.Name, "api") || strings.HasPrefix(res.Name, "oauth") {
			recommendations = append(recommendations, strings.Join([]string{
				"If the API or OAuth FQDNs are not resolving then there is likely an issue with",
				"the external-dns operator on the parent Management Cluster of this HCP cluster.",
				"Check the external-dns operator in the HyperShift namespace to ensure it has valid",
				"AWS credentials and is running.",
			}, " "))
		}
	}
	return recommendations
}

func (v *verifyDNSOptions) renderTable(report dns.DNSVerificationReport) {
	// Print cluster info
	fmt.Fprintf(v.Out, "Cluster: %s (ID: %s, Region: %s)\n\n",
		report.Cluster.Name, report.Cluster.ID, report.Cluster.Region)

	// Print summary at the top with colors
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Fprintf(v.Out, "Summary: Total: %d, Passed: %s, Failed: %s, Skipped: %s\n\n",
		report.Summary.Total,
		green(report.Summary.Passed),
		red(report.Summary.Failed),
		yellow(report.Summary.Skipped))

	// Create and configure table
	table := tablewriter.NewWriter(v.Out)
	table.SetHeader([]string{"DNS Name", "Type", "Status", "Resolved IPs",
		"Actual Target", "Expected Target", "Error/Skip Reason"})
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t")
	table.SetNoWhiteSpace(true)

	// Populate table rows
	for _, res := range report.Results {
		resolvedIPs := strings.Join(res.ResolvedIPs, ", ")
		errorOrSkip := res.ErrorMessage
		if res.SkipReason != "" {
			errorOrSkip = res.SkipReason
		}

		// Color the status based on result
		var coloredStatus string
		switch res.Status {
		case dns.VerifyResultStatusPass:
			coloredStatus = green(string(res.Status))
		case dns.VerifyResultStatusFail:
			coloredStatus = red(string(res.Status))
			// Also color error messages red
			if errorOrSkip != "" {
				errorOrSkip = red(errorOrSkip)
			}
		case dns.VerifyResultStatusSkip:
			coloredStatus = yellow(string(res.Status))
		default:
			coloredStatus = string(res.Status)
		}

		table.Append([]string{
			res.Name,
			string(res.Type),
			coloredStatus,
			resolvedIPs,
			res.ActualTarget,
			res.ExpectedTarget,
			errorOrSkip,
		})
	}

	table.Render()

	// Print recommendations below the table
	if len(report.Recommendations) > 0 {
		fmt.Fprintf(v.Out, "\nRecommendations:\n")
		for i, rec := range report.Recommendations {
			fmt.Fprintf(v.Out, "%d. %s\n", i+1, rec)
		}
	}
}
