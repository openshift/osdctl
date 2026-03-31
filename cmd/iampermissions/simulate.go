package iampermissions

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	cco "github.com/openshift/cloud-credential-operator/pkg/apis/cloudcredential/v1"
	"github.com/openshift/osdctl/pkg/policies"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type simulateOptions struct {
	// Test scenarios (supplementary manifests)
	ManifestFile string
	ManifestDir  string

	// CredentialsRequest-based validation
	ReleaseVersion string
	Cloud          policies.CloudSpec

	// Output options
	OutputFormat string
	OutputFile   string
	Region       string

	// Injected dependencies for testability
	iamClientFunc       func(ctx context.Context, region string) (policies.IAMSimulator, error)
	downloadFunc        func(string, policies.CloudSpec) (string, error)
	managedPoliciesFunc func(string) (string, error)
	outputWriter        io.Writer
}

func newCmdSimulate() *cobra.Command {
	ops := &simulateOptions{
		iamClientFunc:       policies.NewIAMClient,
		downloadFunc:        policies.DownloadCredentialRequests,
		managedPoliciesFunc: policies.DownloadManagedPolicies,
		outputWriter:        os.Stdout,
		OutputFormat:        "table",
		Region:              "us-east-1",
	}

	cmd := &cobra.Command{
		Use:   "simulate",
		Short: "Simulate IAM policies against required permissions to detect mismatches",
		Long: `Simulate validates that ROSA managed IAM policies grant all permissions
required by OCP components. It uses AWS IAM SimulateCustomPolicy to test
each required action against the managed policy, including condition key
contexts that CredentialsRequest diffing alone cannot catch.

Managed policies are automatically fetched from the managed-cluster-config
repository for the corresponding major.minor version, and all managed
policies are compared against the respective operator CredentialsRequests
extracted from the release image.

Examples:
  # Compare all managed policies for a version against operator CRs
  osdctl iampermissions simulate \
    --release-version 4.17.0

  # Also run supplementary test manifests (e.g. for condition key scenarios)
  osdctl iampermissions simulate \
    --release-version 4.17.0 \
    --manifest-file ./ebs-csi-driver.yaml

  # Output JUnit XML for CI integration
  osdctl iampermissions simulate \
    --release-version 4.17.0 \
    --output junit \
    --output-file results.xml`,
		Args:              cobra.ExactArgs(0),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			ops.Cloud = *cmd.Flag(cloudFlagName).Value.(*policies.CloudSpec)
			cmdutil.CheckErr(ops.run())
		},
	}

	cmd.Flags().StringVar(&ops.ManifestFile, "manifest-file", "", "Path to a supplementary test manifest YAML")
	cmd.Flags().StringVar(&ops.ManifestDir, "manifest-dir", "", "Path to a directory of supplementary test manifest YAMLs")
	cmd.Flags().StringVarP(&ops.ReleaseVersion, "release-version", "r", "", "OCP release version (required)")
	cmd.Flags().StringVarP(&ops.OutputFormat, "output", "o", "table", "Output format: table, json, junit")
	cmd.Flags().StringVar(&ops.OutputFile, "output-file", "", "Write output to file instead of stdout")
	cmd.Flags().StringVar(&ops.Region, "region", "us-east-1", "AWS region for IAM API calls")

	_ = cmd.MarkFlagRequired("release-version")

	return cmd
}

func (o *simulateOptions) run() error {
	ctx := context.Background()

	if o.Cloud != policies.AWS {
		return fmt.Errorf("unsupported cloud provider %q: only 'aws' is supported for IAM policy simulation", o.Cloud.String())
	}

	// Create IAM client
	client, err := o.iamClientFunc(ctx, o.Region)
	if err != nil {
		return fmt.Errorf("failed to create IAM client: %w", err)
	}

	// Fetch managed policies from managed-cluster-config
	fmt.Fprintf(os.Stderr, "Fetching managed policies from managed-cluster-config for %s\n", o.ReleaseVersion)
	policyDir, err := o.managedPoliciesFunc(o.ReleaseVersion)
	if err != nil {
		return fmt.Errorf("failed to download managed policies: %w", err)
	}

	policyDocs, err := policies.LoadPoliciesFromDir(policyDir)
	if err != nil {
		return err
	}

	if len(policyDocs) == 0 {
		return fmt.Errorf("no policy documents found for version %s", o.ReleaseVersion)
	}

	var allReports []*policies.SimulationReport

	// Run supplementary manifest scenarios
	if o.ManifestFile != "" || o.ManifestDir != "" {
		reports, err := o.runManifestSimulations(ctx, client, policyDocs)
		if err != nil {
			return err
		}
		allReports = append(allReports, reports...)
	}

	// Run CredentialsRequest-based validation
	reports, err := o.runCredentialsRequestSimulations(ctx, client, policyDocs)
	if err != nil {
		return err
	}
	allReports = append(allReports, reports...)

	if len(allReports) == 0 {
		return fmt.Errorf("no simulations were run")
	}

	// Merge all reports
	merged := policies.MergeReports(allReports...)

	// Write output
	if err := o.writeOutput(merged); err != nil {
		return err
	}

	// Exit with error if any tests failed
	if merged.Failed > 0 {
		return fmt.Errorf("%d/%d policy simulations failed", merged.Failed, merged.Total)
	}

	return nil
}

// runManifestSimulations loads supplementary test manifests and runs simulations.
func (o *simulateOptions) runManifestSimulations(ctx context.Context, client policies.IAMSimulator, policyDocs map[string]string) ([]*policies.SimulationReport, error) {
	var manifests []*policies.SimulationManifest

	if o.ManifestFile != "" {
		m, err := policies.LoadSimulationManifest(o.ManifestFile)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, m)
	}

	if o.ManifestDir != "" {
		entries, err := os.ReadDir(o.ManifestDir)
		if err != nil {
			return nil, fmt.Errorf("failed to read manifest directory %s: %w", o.ManifestDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
				continue
			}
			path := filepath.Join(o.ManifestDir, entry.Name())
			m, err := policies.LoadSimulationManifest(path)
			if err != nil {
				return nil, err
			}
			manifests = append(manifests, m)
		}
	}

	var reports []*policies.SimulationReport

	for _, manifest := range manifests {
		policyJSON, err := findPolicyForManifest(manifest, policyDocs)
		if err != nil {
			return nil, err
		}

		fmt.Fprintf(os.Stderr, "Simulating %s against %s (%d scenarios)\n",
			manifest.Component, manifest.PolicyName, len(manifest.Scenarios))

		report, err := policies.SimulateManifest(ctx, client, policyJSON, manifest)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// runCredentialsRequestSimulations extracts CredentialsRequests from a release image
// and simulates them against the managed policies.
func (o *simulateOptions) runCredentialsRequestSimulations(ctx context.Context, client policies.IAMSimulator, policyDocs map[string]string) ([]*policies.SimulationReport, error) {
	fmt.Fprintf(os.Stderr, "Downloading CredentialsRequests for %s\n", o.ReleaseVersion)
	dir, err := o.downloadFunc(o.ReleaseVersion, o.Cloud)
	if err != nil {
		return nil, fmt.Errorf("failed to download CredentialsRequests: %w", err)
	}

	crs, err := policies.ParseCredentialsRequestsInDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CredentialsRequests: %w", err)
	}

	var reports []*policies.SimulationReport

	for _, cr := range crs {
		awsSpec, err := policies.GetAWSProviderSpec(cr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: not an AWS CredentialsRequest\n", cr.Name)
			continue
		}

		// Convert StatementEntries to simulation inputs
		var entries []policies.StatementEntrySimInput
		for _, stmt := range awsSpec.StatementEntries {
			if strings.ToLower(stmt.Effect) != "allow" {
				continue
			}
			resources := []string{"*"}
			if stmt.Resource != "" && stmt.Resource != "*" {
				resources = []string{stmt.Resource}
			}

			entry := policies.StatementEntrySimInput{
				Actions:   stmt.Action,
				Resources: resources,
			}

			// Convert PolicyCondition to ContextKeyDefs if present
			if len(stmt.PolicyCondition) > 0 {
				entry.Context = convertPolicyConditionToContext(stmt.PolicyCondition)
			}

			entries = append(entries, entry)
		}

		// Find a matching policy using the managed-cluster-config naming convention.
		derivedKey := policies.CRToManagedPolicyKey(cr.Spec.SecretRef.Namespace, cr.Name)
		policyJSON, ok := policyDocs[derivedKey]
		if !ok {
			fmt.Fprintf(os.Stderr, "Skipping %s: no matching policy found for key %q\n", cr.Name, derivedKey)
			continue
		}

		fmt.Fprintf(os.Stderr, "Simulating CredentialsRequest %s (%d statements)\n", cr.Name, len(entries))

		report, err := policies.SimulateCredentialsRequest(ctx, client, policyJSON, cr.Name, entries)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}

	return reports, nil
}

// findPolicyForManifest finds the policy document that matches a simulation manifest.
func findPolicyForManifest(manifest *policies.SimulationManifest, policyDocs map[string]string) (string, error) {
	if manifest.PolicyName == "" {
		return "", fmt.Errorf("manifest for component %q has no policyName specified", manifest.Component)
	}

	if doc, ok := policyDocs[manifest.PolicyName]; ok {
		return doc, nil
	}

	return "", fmt.Errorf("no matching policy found for manifest %s (policyName: %q). Available policies: %v",
		manifest.Component, manifest.PolicyName, policyDocNames(policyDocs))
}

func policyDocNames(docs map[string]string) []string {
	var names []string
	for name := range docs {
		names = append(names, name)
	}
	return names
}

// convertPolicyConditionToContext converts a CredentialsRequest IAMPolicyCondition
// to ContextKeyDefs for simulation.
func convertPolicyConditionToContext(condition cco.IAMPolicyCondition) map[string]policies.ContextKeyDef {
	contextDefs := make(map[string]policies.ContextKeyDef)

	for _, keyValues := range condition {
		for key, val := range keyValues {
			def := policies.ContextKeyDef{
				Type: "string",
			}
			switch v := val.(type) {
			case string:
				def.Values = []string{v}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						def.Values = append(def.Values, s)
					}
				}
			}
			contextDefs[key] = def
		}
	}

	return contextDefs
}

// writeOutput writes the simulation report in the requested format.
func (o *simulateOptions) writeOutput(report *policies.SimulationReport) error {
	w := o.outputWriter
	if o.OutputFile != "" {
		f, err := os.Create(o.OutputFile)
		if err != nil {
			return fmt.Errorf("failed to create output file %s: %w", o.OutputFile, err)
		}
		defer f.Close()
		w = f
	}

	switch o.OutputFormat {
	case "table":
		report.FormatTable(w)
	case "json":
		if err := report.FormatJSON(w); err != nil {
			return fmt.Errorf("failed to write JSON output: %w", err)
		}
	case "junit":
		if err := report.FormatJUnitXML(w); err != nil {
			return fmt.Errorf("failed to write JUnit XML output: %w", err)
		}
	default:
		return fmt.Errorf("unsupported output format: %s (supported: table, json, junit)", o.OutputFormat)
	}

	return nil
}
