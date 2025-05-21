package cloudtrail

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	ctUtil "github.com/openshift/osdctl/cmd/cloudtrail/pkg"
	ctAws "github.com/openshift/osdctl/cmd/cloudtrail/pkg/aws"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

/*
	Whenever cloud tail command it used.

	opts := store address of the struct
	permissionDeniedCmd := runs every variable in the string array of *cobra.Command
		if there are error will return error message
		else:
			return permissionDeniedCmd which is the result.

	func isforbiddentevent used for non-priviledged/not useable commands/request
*/

type permissionDeniedEventsOptions struct {
	ClusterID string
	StartTime string
	PrintUrl  bool
	PrintRaw  bool
}

func newCmdPermissionDenied() *cobra.Command {
	opts := &permissionDeniedEventsOptions{} //Stores address of struct into opts

	permissionDeniedCmd := &cobra.Command{
		Use:   "permission-denied-events",
		Short: "Prints cloudtrail permission-denied events to console.",
		RunE: func(cmd *cobra.Command, args []string) error {
			//runs run() for errorchecking
			return opts.run()
		},
	}

	/*
		Assign the flags
		StringvarP accepts Shorthand letter for single -
		 	pointer:*string, name:string, shorthand:string, value:string, usage:string
		BoolVarP -> essentially the same thing
		 	pointer:*string, name:string, shorthand:string, value:bool, usage:string

		Default values are shown below if no modifications
	*/
	permissionDeniedCmd.Flags().StringVarP(&opts.ClusterID, "cluster-id", "C", "", "Cluster ID")
	permissionDeniedCmd.Flags().StringVarP(&opts.StartTime, "since", "", "5m", "Specifies that only events that occur within the specified time are returned.Defaults to 5m. Valid time units are \"ns\", \"us\" (or \"µs\"), \"ms\", \"s\", \"m\", \"h\".")
	permissionDeniedCmd.Flags().BoolVarP(&opts.PrintUrl, "url", "u", false, "Generates Url link to cloud console cloudtrail event")
	permissionDeniedCmd.Flags().BoolVarP(&opts.PrintRaw, "raw-event", "r", false, "Prints the cloudtrail events to the console in raw json format")
	permissionDeniedCmd.MarkFlagRequired("cluster-id") //invokes error if performed w/o flag
	return permissionDeniedCmd
}

func isforbiddenEvent(event types.Event) (bool, error) {
	// Checks if there exist a Client.UnauthorizedOperation and return error if true
	permissionDeniedErrorRegexp := ".*Client.UnauthorizedOperation.*"

	check, err := regexp.Compile(permissionDeniedErrorRegexp)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}
	raw, err := ctAws.ExtractUserDetails(event.CloudTrailEvent)
	if err != nil {
		return false, fmt.Errorf("[ERROR] failed to extract raw CloudTrail event details: %w", err)
	}
	errorCode := raw.ErrorCode
	if errorCode != "" && check.MatchString(errorCode) {
		return true, nil
	}

	return false, nil
}
func (p *permissionDeniedEventsOptions) run() error {

	// check for valid cluster key
	err := utils.IsValidClusterKey(p.ClusterID)
	if err != nil {
		return err
	}

	// check connection
	connection, err := utils.CreateConnection()
	if err != nil {
		return fmt.Errorf("unable to create connection to ocm: %w", err)
	}
	defer connection.Close()

	// See status of cluster
	cluster, err := utils.GetClusterAnyStatus(connection, p.ClusterID)
	if err != nil {
		return err
	}

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return fmt.Errorf("[ERROR] this command is only available for AWS clusters")
	}

	//cfg?
	cfg, err := osdCloud.CreateAWSV2Config(connection, cluster)
	if err != nil {
		return err
	}

	//Start time
	startTime, err := ctUtil.ParseDurationToUTC(p.StartTime)
	if err != nil {
		return err
	}

	//arn?
	arn, accountId, err := ctAws.Whoami(*sts.NewFromConfig(cfg))
	if err != nil {
		return err
	}
	fmt.Printf("[INFO] Checking Permission Denied History since %v for AWS Account %v as %v \n", startTime, accountId, arn)
	cloudTrailclient := cloudtrail.NewFromConfig(cfg)
	fmt.Printf("[INFO] Fetching %v Event History...", cfg.Region)
	lookupOutput, err := ctAws.GetEventsP(cloudTrailclient, startTime, false)
	if err != nil {
		return err
	}

	filteredEvents, err := ctUtil.ApplyFilters(lookupOutput,
		func(event types.Event) (bool, error) {
			return isforbiddenEvent(event)
		},
	)
	if err != nil {
		return err
	}

	ctUtil.PrintEvents(filteredEvents, p.PrintUrl, p.PrintRaw)
	// Region
	if DefaultRegion != cfg.Region {
		defaultConfig, err := config.LoadDefaultConfig(
			context.Background(),
			config.WithRegion(DefaultRegion))
		if err != nil {
			return err
		}
		// ???
		defaultCloudtrailClient := cloudtrail.New(cloudtrail.Options{
			Region:      DefaultRegion,
			Credentials: cfg.Credentials,
			HTTPClient:  cfg.HTTPClient,
		})
		fmt.Printf("[INFO] Fetching Cloudtrail Global Permission Denied Event History from %v Region...", defaultConfig.Region)
		lookupOutput, err := ctAws.GetEventsP(defaultCloudtrailClient, startTime, false)
		if err != nil {
			return err
		}
		filteredEvents, err := ctUtil.ApplyFilters(lookupOutput,
			func(event types.Event) (bool, error) {
				return isforbiddenEvent(event)
			},
		)
		if err != nil {
			return err
		}
		ctUtil.PrintEvents(filteredEvents, p.PrintUrl, p.PrintRaw)
	}

	return err

}
