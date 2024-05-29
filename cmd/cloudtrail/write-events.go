package cloudtrail

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ctUtil "github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	ctAws "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	envConfig "github.com/openshift/osdctl/pkg/envConfig"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

var DefaultRegion = "us-east-1"

// LookupEventsOptions struct for holding options for event lookup
type LookupEventsOptions struct {
	clusterID      string
	startTime      string
	printEventUrl  bool
	printRawEvents bool
	printAllEvents bool
}

// RawEventDetails struct represents the structure of an AWS raw event
type RawEventDetails struct {
	EventVersion string `json:"eventVersion"`
	UserIdentity struct {
		AccountId      string `json:"accountId"`
		SessionContext struct {
			SessionIssuer struct {
				Type     string `json:"type"`
				UserName string `json:"userName"`
				Arn      string `json:"arn"`
			} `json:"sessionIssuer"`
		} `json:"sessionContext"`
	} `json:"userIdentity"`
	EventRegion string `json:"awsRegion"`
	EventId     string `json:"eventID"`
}

func newCmdWriteEvents() *cobra.Command {
	ops := &LookupEventsOptions{}
	listEventsCmd := &cobra.Command{
		Use:   "write-events",
		Short: "Prints cloudtrail write events to console with optional filtering",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.run()
		},
	}
	listEventsCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.startTime, "since", "", "1h", "Specifies that only events that occur within the specified time are returned.Defaults to 1h.Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	listEventsCmd.Flags().BoolVarP(&ops.printEventUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	listEventsCmd.Flags().BoolVarP(&ops.printRawEvents, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	listEventsCmd.Flags().BoolVarP(&ops.printAllEvents, "all", "A", false, "Prints all cloudtrail write events without filtering")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}

func (o *LookupEventsOptions) run() error {

	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	cluster, err := utils.GetClusterAnyStatus(connection, o.clusterID)
	if err != nil {
		return err
	}
	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("[ERROR] this command is only available for AWS clusters")
	}

	Ignore, err := envConfig.LoadCloudTrailConfig()
	if err != nil {
		return fmt.Errorf("[ERROR] error Loading cloudtrail configuration file: %w", err)
	}
	if len(Ignore) == 0 {
		fmt.Println("\n[WARNING] No filter list detected! If you want intend to apply user filtering for the cloudtrail events, please add cloudtrail_cmd_lists to your osdctl configuration file.")

	}

	mergedRegex := ctUtil.MergeRegex(Ignore)
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}
	DefaultRegion := "us-east-1"
	startTime, err := ctUtil.ParseDurationToUTC(o.startTime)

	if err != nil {
		return err
	}

	fetchFilterPrintEvents := func(client cloudtrail.Client, startTime time.Time, o *LookupEventsOptions) error {
		lookupOutput, err := ctAws.GetEvents(startTime, &client)
		if err != nil {
			return err
		}
		if o.printAllEvents {
			mergedRegex = ""
		}
		filteredEvents := ctUtil.Filters[2](lookupOutput, mergedRegex)
		ctUtil.PrintEvents(*filteredEvents, o.printEventUrl, o.printRawEvents)
		fmt.Println("")
		return err
	}

	arn, accountId, err := ctAws.Whoami(*sts.NewFromConfig(cfg))
	fmt.Printf("[INFO] Checking write event history since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)
	if err := fetchFilterPrintEvents(*cloudTrailclient, startTime, o); err != nil {
		return err
	}

	if DefaultRegion != cfg.Region {
		defaultConfig, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithRegion(DefaultRegion))
		if err != nil {
			return err
		}

		defaultCloudtrailClient := cloudtrail.New(cloudtrail.Options{
			Region:      DefaultRegion,
			Credentials: cfg.Credentials,
			HTTPClient:  cfg.HTTPClient,
		})
		fmt.Printf("[INFO] Fetching Cloudtrail Global Event History from %v Region...", defaultConfig.Region)
		if err := fetchFilterPrintEvents(*defaultCloudtrailClient, startTime, o); err != nil {
			return err
		}
	}

	return err
}
