package cloudtrail

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

// Default error patterns to match for IAM/permission issues
var defaultErrorPatterns = []string{
	"AccessDenied",
	"UnauthorizedOperation",
	"Client.UnauthorizedOperation",
	"Forbidden",
	"InvalidClientTokenId",
	"AuthFailure",
	"ExpiredToken",
	"SignatureDoesNotMatch",
}

type errorsOptions struct {
	ClusterID  string
	StartTime  string
	PrintUrl   bool
	PrintRaw   bool
	JSONOutput bool
	ErrorTypes []string
}

type errorEventOutput struct {
	EventName   string `json:"eventName"`
	EventTime   string `json:"eventTime"`
	ErrorCode   string `json:"errorCode"`
	UserARN     string `json:"userArn,omitempty"`
	UserName    string `json:"userName,omitempty"`
	Region      string `json:"region,omitempty"`
	ConsoleLink string `json:"consoleLink,omitempty"`
}

func newCmdErrors() *cobra.Command {
	opts := &errorsOptions{}

	errorsCmd := &cobra.Command{
		Use:   "errors",
		Short: "Prints CloudTrail error events (permission/IAM issues) to console.",
		Long: `Surfaces permission and IAM-related errors from AWS CloudTrail.

By default, matches these error patterns:
  - AccessDenied
  - UnauthorizedOperation / Client.UnauthorizedOperation
  - Forbidden
  - InvalidClientTokenId
  - AuthFailure
  - ExpiredToken
  - SignatureDoesNotMatch

Use --error-types to filter for specific error patterns.`,
		Example: `  # Check for permission errors in the last hour
  osdctl cloudtrail errors -C <cluster-id> --since 1h

  # Check for specific error types only
  osdctl cloudtrail errors -C <cluster-id> --error-types AccessDenied,Forbidden

  # Output as JSON for scripting
  osdctl cloudtrail errors -C <cluster-id> --json

  # Include console links for each event
  osdctl cloudtrail errors -C <cluster-id> --url`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return opts.run()
		},
	}

	errorsCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "C", "", "Cluster ID")
	errorsCmd.Flags().StringVarP(&opts.StartTime, "since", "", "1h", "Time window to search (e.g., 30m, 1h, 24h). Valid units: ns, us, ms, s, m, h.")
	errorsCmd.Flags().BoolVarP(&opts.PrintUrl, "url", "u", false, "Include console URL links for each event")
	errorsCmd.Flags().BoolVarP(&opts.PrintRaw, "raw-event", "r", false, "Print raw CloudTrail event JSON")
	errorsCmd.Flags().BoolVar(&opts.JSONOutput, "json", false, "Output results as JSON")
	errorsCmd.Flags().StringSliceVar(&opts.ErrorTypes, "error-types", nil, "Comma-separated list of error patterns to match (default: all common permission errors)")
	_ = errorsCmd.MarkFlagRequired("cluster-id")

	return errorsCmd
}

func (o *errorsOptions) run() error {
	err := utils.IsValidClusterKey(o.ClusterID)
	if err != nil {
		return err
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to OCM: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.ClusterID)
	if err != nil {
		return err
	}

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("this command is only available for AWS clusters")
	}

	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	startTime, err := parseDurationToUTC(o.StartTime)
	if err != nil {
		return err
	}

	arn, accountID, err := Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}

	// Build error patterns to match
	patterns := defaultErrorPatterns
	if len(o.ErrorTypes) > 0 {
		patterns = o.ErrorTypes
	}

	if !o.JSONOutput {
		fmt.Printf("[INFO] Checking error history since %v for AWS Account %v as %v\n", startTime.Format(time.RFC3339), accountID, arn)
		fmt.Printf("[INFO] Matching error patterns: %v\n", patterns)
		fmt.Printf("[INFO] Fetching CloudTrail error events from %v region...\n", cfg.Region)
	}

	awsAPI := NewEventAPI(cfg, false, cfg.Region)
	requestTime := Period{StartTime: startTime, EndTime: time.Now().UTC()}
	generator := awsAPI.GetEvents(o.ClusterID, requestTime)

	var allEvents []errorEventOutput
	eventCount := 0

	// Process events from cluster region
	for page := range generator {
		filteredEvents, err := ApplyFilters(page.AWSEvent,
			func(event types.Event) (bool, error) {
				return o.isErrorEvent(event, patterns)
			},
		)
		if err != nil {
			return err
		}

		if o.JSONOutput {
			for _, event := range filteredEvents {
				output := o.eventToOutput(event, cfg.Region)
				allEvents = append(allEvents, output)
			}
		} else if o.PrintRaw {
			for _, event := range filteredEvents {
				if event.CloudTrailEvent != nil {
					fmt.Println(*event.CloudTrailEvent)
				}
			}
		} else if len(filteredEvents) > 0 {
			o.printEvents(filteredEvents, cfg.Region)
		}
		eventCount += len(filteredEvents)
	}

	// Also check global region if different
	if DEFAULT_REGION != cfg.Region {
		defaultAwsAPI := NewEventAPI(cfg, true, DEFAULT_REGION)

		if !o.JSONOutput && !o.PrintRaw {
			fmt.Printf("[INFO] Fetching CloudTrail error events from %v region...\n", DEFAULT_REGION)
		}

		generator := defaultAwsAPI.GetEvents(o.ClusterID, requestTime)

		for page := range generator {
			filteredEvents, err := ApplyFilters(page.AWSEvent,
				func(event types.Event) (bool, error) {
					return o.isErrorEvent(event, patterns)
				},
			)
			if err != nil {
				return err
			}

			if o.JSONOutput {
				for _, event := range filteredEvents {
					output := o.eventToOutput(event, DEFAULT_REGION)
					allEvents = append(allEvents, output)
				}
			} else if o.PrintRaw {
				for _, event := range filteredEvents {
					if event.CloudTrailEvent != nil {
						fmt.Println(*event.CloudTrailEvent)
					}
				}
			} else if len(filteredEvents) > 0 {
				o.printEvents(filteredEvents, DEFAULT_REGION)
			}
			eventCount += len(filteredEvents)
		}
	}

	if o.JSONOutput {
		output, err := json.MarshalIndent(allEvents, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal JSON output: %w", err)
		}
		fmt.Println(string(output))
	} else {
		fmt.Printf("\n[INFO] Found %d error event(s)\n", eventCount)
	}

	return nil
}

func (o *errorsOptions) isErrorEvent(event types.Event, patterns []string) (bool, error) {
	raw, err := ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return false, fmt.Errorf("failed to extract CloudTrail event details: %w", err)
	}

	errorCode := raw.ErrorCode
	if errorCode == "" {
		return false, nil
	}

	for _, pattern := range patterns {
		check, err := regexp.Compile("(?i)" + regexp.QuoteMeta(pattern))
		if err != nil {
			return false, fmt.Errorf("failed to compile regex for pattern %s: %w", pattern, err)
		}
		if check.MatchString(errorCode) {
			return true, nil
		}
	}

	return false, nil
}

func (o *errorsOptions) eventToOutput(event types.Event, region string) errorEventOutput {
	output := errorEventOutput{
		Region: region,
	}

	if event.EventName != nil {
		output.EventName = *event.EventName
	}
	if event.EventTime != nil {
		output.EventTime = event.EventTime.Format(time.RFC3339)
	}

	raw, err := ExtractUserDetails(event.CloudTrailEvent)
	if err == nil {
		output.ErrorCode = raw.ErrorCode
		output.UserARN = raw.UserIdentity.SessionContext.SessionIssuer.Arn
		output.UserName = raw.UserIdentity.SessionContext.SessionIssuer.UserName
	}

	if o.PrintUrl && event.EventId != nil {
		output.ConsoleLink = fmt.Sprintf("https://%s.console.aws.amazon.com/cloudtrailv2/home?region=%s#/events/%s",
			region, region, *event.EventId)
	}

	return output
}

func (o *errorsOptions) printEvents(events []types.Event, region string) {
	for _, event := range events {
		fmt.Println("─────────────────────────────────────────────────────────────")

		if event.EventName != nil {
			fmt.Printf("Event: %s\n", *event.EventName)
		}
		if event.EventTime != nil {
			fmt.Printf("Time:  %s\n", event.EventTime.Format(time.RFC3339))
		}

		raw, err := ExtractUserDetails(event.CloudTrailEvent)
		if err == nil {
			if raw.ErrorCode != "" {
				fmt.Printf("Error: %s\n", raw.ErrorCode)
			}
			userName := raw.UserIdentity.SessionContext.SessionIssuer.UserName
			if userName != "" {
				fmt.Printf("User:  %s\n", userName)
			}
			userArn := raw.UserIdentity.SessionContext.SessionIssuer.Arn
			if userArn != "" {
				fmt.Printf("ARN:   %s\n", userArn)
			}
		}

		fmt.Printf("Region: %s\n", region)

		if o.PrintUrl && event.EventId != nil {
			fmt.Printf("Console: https://%s.console.aws.amazon.com/cloudtrailv2/home?region=%s#/events/%s\n",
				region, region, *event.EventId)
		}
	}
}

func parseDurationToUTC(since string) (time.Time, error) {
	duration, err := time.ParseDuration(since)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid duration format %q: %w", since, err)
	}
	if duration <= 0 {
		return time.Time{}, fmt.Errorf("duration must be positive, got %q", since)
	}
	return time.Now().UTC().Add(-duration), nil
}
