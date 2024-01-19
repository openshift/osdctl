package cloudtrail

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type Config struct {
	Igno_list []string `mapstructure: regex_igno_list`
}
type LookupEventsoptions struct {
	clusterID string
	since     string
	pages     int
	cluster   *cmv1.Cluster
}

func newwrite_eventsCmd() *cobra.Command {
	ops := newWrite_eventsOptions()
	listEventsCmd := &cobra.Command{
		Use:   "write-events",
		Short: "Prints out all cloudtrail write events to console",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("")
			cmdutil.CheckErr(ops.complete(cmd, args))

		},
	}
	listEventsCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.since, "since", "", "", "Duration of lookup")
	listEventsCmd.Flags().IntVar(&ops.pages, "pages", 50, "Command will display X pages of Cloud Trail logs for the cluster. Pages is set to 40 by default")
	listEventsCmd.MarkFlagRequired("cluster-id")
	return listEventsCmd
}
func newWrite_eventsOptions() *LookupEventsoptions {

	return &LookupEventsoptions{}

}

func (o *LookupEventsoptions) complete(cmd *cobra.Command, _ []string) error {
	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer connection.Close()

	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}

	o.cluster = cluster

	o.clusterID = cluster.ID()

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return errors.New("this command is only available for AWS clusters")
	}
	/*
		Ideally we would want additional validation here for:
		- the machine type exists
		- the node exists on the cluster

		As this command is idempotent, it will just fail on a later stage if e.g. the
		machine type doesn't exist and can be re-run.
	*/

	return nil
}
func parseDurationToUTC(input string) (time.Time, error) {
	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, err
	}
	return time.Now().UTC().Add(-duration), nil
}
func GetARN(awsClient sts.Client) (string, error) {

	ctx := context.TODO()
	callerIdentityOutput, err := awsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	userArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", err
	}

	return userArn.String(), nil
}

func GetUserID(awsClient sts.Client) (string, error) {

	ctx := context.TODO()
	callerIdentityOutput, err := awsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return arn.ARN{}.AccountID, err
	}
	userID, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return userID.AccountID, err
	}

	return userID.AccountID, nil
}
func GetEvents(since string, client *cloudtrail.Client, pages int) ([]*cloudtrail.LookupEventsOutput, error) {
	ctx := context.TODO()
	starttime, err := parseDurationToUTC(since)

	if err != nil {
		return nil, err
	}
	cloudtrailClient := client
	if err != nil {
		return nil, err
	}

	Events := []*cloudtrail.LookupEventsOutput{}
	var input = cloudtrail.LookupEventsInput{StartTime: &starttime}

	if err != nil {
		return nil, err
	}
	maxPages := pages
	fmt.Printf("Checking write event history since %s for AWS Account (&your account ID) as (&UserARN) \n\n", starttime)
	for counter := 0; counter <= maxPages; counter++ {

		cloudTrailEvents, err := cloudtrailClient.LookupEvents(ctx, &input)
		if err != nil {
			return nil, err
		}

		Events = append(Events, cloudTrailEvents)

		input.NextToken = cloudTrailEvents.NextToken
		if cloudTrailEvents.NextToken == nil {
			break
		}

	}

	return Events, nil
}

func printEvents(filteredEvents []types.Event) {

	for _, event := range filteredEvents {
		// Print the relevant information from the event

		if event.EventName != nil {
			fmt.Printf("%s |", *event.EventName)
		} else {
			fmt.Println("<not available> |")
		}

		if event.EventTime != nil {
			fmt.Printf("%s |", event.EventTime.String())
		} else {
			fmt.Println("<not available> |")
		}
		if event.Username != nil {
			fmt.Printf("User: %s\n", *event.Username)
		} else {
			fmt.Println("User: <not available>")
		}

	}

}

func filterUsers(lookupOutputs []*cloudtrail.LookupEventsOutput) (*[]types.Event, error) {
	filteredEvents := []types.Event{}
	var myconfig Config

	err := viper.Unmarshal(&myconfig)
	if err != nil {
		fmt.Println("Error Umashalng config into struct", err)
		return &filteredEvents, nil
	}

	ignoreRegexList := myconfig.Igno_list

	for _, lookupOutput := range lookupOutputs {
		for _, event := range lookupOutput.Events {

			matched := false
			for _, regStr := range ignoreRegexList {
				regex, err := regexp.Compile(regStr)
				if err != nil {
					fmt.Println("Error Compling Regex")
					return &filteredEvents, err
				}
				if event.Username != nil {
					if regex.MatchString(*event.Username) {
						matched = true
						break
					}
				} else {
					continue
				}

			}
			if !matched {
				filteredEvents = append(filteredEvents, event)

			}

		}
	}

	return &filteredEvents, nil
}

func (o *LookupEventsoptions) run(cmd *cobra.Command, _ []string) error {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	if err != nil {
		return err
	}
	cfg, err := osdCloud.CreateAWSV2Config(ocmClient, o.cluster)
	if err != nil {
		fmt.Println("Could Not get cfg!")
		return err
	}

	cloudtrailClient := cloudtrail.NewFromConfig(cfg)

	lookupOutput, err := GetEvents(o.since, cloudtrailClient, o.pages)

	if err != nil {
		return err
	}
	fmt.Printf("\n\n[+] Fetching %s Event History..\n", cfg.Region)
	filtered, err := filterUsers(lookupOutput)
	printEvents(*filtered)

	fmt.Println("")
	return err
}
