package policies

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// SimulationResult holds the result of simulating a single action against a policy.
type SimulationResult struct {
	Component        string   `json:"component"`
	Action           string   `json:"action"`
	Resource         string   `json:"resource"`
	ContextDesc      string   `json:"contextDescription,omitempty"`
	Decision         string   `json:"decision"`
	Expected         string   `json:"expected"`
	Pass             bool     `json:"pass"`
	MissingContext   []string `json:"missingContextValues,omitempty"`
	MatchedStatement string   `json:"matchedStatement,omitempty"`
}

// SimulationReport holds all results for a validation run.
type SimulationReport struct {
	PolicyName string             `json:"policyName"`
	Results    []SimulationResult `json:"results"`
	Passed     int                `json:"passed"`
	Failed     int                `json:"failed"`
	Total      int                `json:"total"`
}

// SimulationScenario defines a single test case for policy simulation.
type SimulationScenario struct {
	Name        string                       `json:"name" yaml:"name"`
	Description string                       `json:"description,omitempty" yaml:"description,omitempty"`
	Action      string                       `json:"action" yaml:"action"`
	Resources   []string                     `json:"resources" yaml:"resources"`
	Context     map[string]ContextKeyDef     `json:"context,omitempty" yaml:"context,omitempty"`
	Expect      string                       `json:"expect" yaml:"expect"`
}

// ContextKeyDef defines a condition context key for simulation.
type ContextKeyDef struct {
	Type   string   `json:"type" yaml:"type"`
	Values []string `json:"values" yaml:"values"`
}

// SimulationManifest defines supplementary test scenarios for a component.
type SimulationManifest struct {
	Component  string               `json:"component" yaml:"component"`
	PolicyName string               `json:"policyName" yaml:"policyName"`
	Scenarios  []SimulationScenario `json:"scenarios" yaml:"scenarios"`
}

// IAMSimulator defines the interface for IAM policy simulation.
// Using an interface allows mocking in unit tests.
type IAMSimulator interface {
	SimulateCustomPolicy(ctx context.Context, params *iam.SimulateCustomPolicyInput, optFns ...func(*iam.Options)) (*iam.SimulateCustomPolicyOutput, error)
}

// NewIAMClient creates a real AWS IAM client using the default credential chain.
func NewIAMClient(ctx context.Context, region string) (IAMSimulator, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return iam.NewFromConfig(cfg), nil
}

// LoadPolicyDocument reads an IAM policy JSON file and returns it as a string.
func LoadPolicyDocument(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read policy file %s: %w", path, err)
	}

	// Validate it's valid JSON
	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("invalid JSON in policy file %s: %w", path, err)
	}

	return string(data), nil
}

// LoadSimulationManifest reads a supplementary test manifest YAML file.
func LoadSimulationManifest(path string) (*SimulationManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest file %s: %w", path, err)
	}

	manifest := &SimulationManifest{}
	if err := yaml.NewYAMLOrJSONDecoder(
		bytes.NewReader(data), len(data),
	).Decode(manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest file %s: %w", path, err)
	}

	return manifest, nil
}

// SimulateScenario runs a single simulation scenario against a policy document.
func SimulateScenario(ctx context.Context, client IAMSimulator, policyJSON string, scenario SimulationScenario, component string) (SimulationResult, error) {
	input := &iam.SimulateCustomPolicyInput{
		PolicyInputList: []string{policyJSON},
		ActionNames:     []string{scenario.Action},
	}

	if len(scenario.Resources) > 0 {
		input.ResourceArns = scenario.Resources
	}

	if len(scenario.Context) > 0 {
		var contextEntries []iamtypes.ContextEntry
		for keyName, keyDef := range scenario.Context {
			contextType, err := parseContextKeyType(keyDef.Type)
			if err != nil {
				return SimulationResult{}, fmt.Errorf("invalid context key type for %s: %w", keyName, err)
			}
			contextEntries = append(contextEntries, iamtypes.ContextEntry{
				ContextKeyName:   aws.String(keyName),
				ContextKeyType:   contextType,
				ContextKeyValues: keyDef.Values,
			})
		}
		input.ContextEntries = contextEntries
	}

	output, err := client.SimulateCustomPolicy(ctx, input)
	if err != nil {
		return SimulationResult{}, fmt.Errorf("SimulateCustomPolicy failed for %s: %w", scenario.Action, err)
	}

	result := SimulationResult{
		Component:   component,
		Action:      scenario.Action,
		ContextDesc: scenario.Name,
		Expected:    scenario.Expect,
	}

	if len(scenario.Resources) > 0 {
		result.Resource = scenario.Resources[0]
	}

	if len(output.EvaluationResults) > 0 {
		evalResult := output.EvaluationResults[0]
		result.Decision = string(evalResult.EvalDecision)
		result.MissingContext = evalResult.MissingContextValues

		if len(evalResult.MatchedStatements) > 0 {
			stmt := evalResult.MatchedStatements[0]
			if stmt.SourcePolicyId != nil {
				result.MatchedStatement = *stmt.SourcePolicyId
			}
		}
	}

	result.Pass = decisionMatchesExpected(result.Decision, result.Expected)

	return result, nil
}

// SimulateManifest runs all scenarios in a manifest against a policy document.
func SimulateManifest(ctx context.Context, client IAMSimulator, policyJSON string, manifest *SimulationManifest) (*SimulationReport, error) {
	report := &SimulationReport{
		PolicyName: manifest.PolicyName,
	}

	for _, scenario := range manifest.Scenarios {
		result, err := SimulateScenario(ctx, client, policyJSON, scenario, manifest.Component)
		if err != nil {
			return nil, err
		}

		report.Results = append(report.Results, result)
		report.Total++
		if result.Pass {
			report.Passed++
		} else {
			report.Failed++
		}
	}

	return report, nil
}

// SimulateCredentialsRequest generates scenarios from a CredentialsRequest's StatementEntries
// and runs them against a policy document. This validates that the managed policy
// grants at least the actions declared in the CredentialsRequest.
func SimulateCredentialsRequest(ctx context.Context, client IAMSimulator, policyJSON string, crName string, entries []StatementEntrySimInput) (*SimulationReport, error) {
	report := &SimulationReport{
		PolicyName: crName,
	}

	for _, entry := range entries {
		for _, action := range entry.Actions {
			scenario := SimulationScenario{
				Name:      fmt.Sprintf("CredentialsRequest %s: %s", crName, action),
				Action:    action,
				Resources: entry.Resources,
				Context:   entry.Context,
				Expect:    "allowed",
			}

			result, err := SimulateScenario(ctx, client, policyJSON, scenario, crName)
			if err != nil {
				return nil, err
			}

			report.Results = append(report.Results, result)
			report.Total++
			if result.Pass {
				report.Passed++
			} else {
				report.Failed++
			}
		}
	}

	return report, nil
}

// StatementEntrySimInput is a simplified input derived from a CredentialsRequest's StatementEntry.
type StatementEntrySimInput struct {
	Actions   []string
	Resources []string
	Context   map[string]ContextKeyDef
}

// decisionMatchesExpected checks if the IAM simulation decision matches what we expected.
func decisionMatchesExpected(decision, expected string) bool {
	switch expected {
	case "allowed":
		return decision == "allowed"
	case "denied":
		return decision == "implicitDeny" || decision == "explicitDeny"
	default:
		return decision == expected
	}
}

// parseContextKeyType converts a string type to the IAM ContextKeyType enum.
func parseContextKeyType(t string) (iamtypes.ContextKeyTypeEnum, error) {
	switch t {
	case "string", "":
		return iamtypes.ContextKeyTypeEnumString, nil
	case "stringList":
		return iamtypes.ContextKeyTypeEnumStringList, nil
	case "numeric":
		return iamtypes.ContextKeyTypeEnumNumeric, nil
	case "numericList":
		return iamtypes.ContextKeyTypeEnumNumericList, nil
	case "boolean":
		return iamtypes.ContextKeyTypeEnumBoolean, nil
	case "booleanList":
		return iamtypes.ContextKeyTypeEnumBooleanList, nil
	case "ip":
		return iamtypes.ContextKeyTypeEnumIp, nil
	case "ipList":
		return iamtypes.ContextKeyTypeEnumIpList, nil
	case "binary":
		return iamtypes.ContextKeyTypeEnumBinary, nil
	case "binaryList":
		return iamtypes.ContextKeyTypeEnumBinaryList, nil
	case "date":
		return iamtypes.ContextKeyTypeEnumDate, nil
	case "dateList":
		return iamtypes.ContextKeyTypeEnumDateList, nil
	default:
		return "", fmt.Errorf("unsupported context key type: %s", t)
	}
}

