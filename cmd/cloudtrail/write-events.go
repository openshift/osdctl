package cloudtrail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail/types"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/savioxavier/termlink"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// type Configuration struct {
// 	prod string
// }

type Configuration struct {
	AwsProxy string `mapstructure:"aws_proxy"`
	Ignore   struct {
		Users []string `mapstructure:"users_with"`
	} `mapstructure:"ignore"`
}

type LookupEventsoptions struct {
	clusterID string
	since     string
	pages     int
	cluster   *cmv1.Cluster
	url       bool
}

type RawEventDetails struct {
	EventVersion string `json:"eventVersion"`
	AccountId    string `json:"accountId"`
	UserIdentity struct {
		Type           string `json:"type"`
		SessionContext struct {
			SessionIssuer struct {
				Type     string `json:"type"`
				UserName string `json:"userName"`
			} `json:"sessionIssuer"`
		} `json:"sessionContext"`
	} `json:"userIdentity"`
	EventRegion string `json:"awsRegion"`
	EventId     string `json:"eventID"`
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
			cmdutil.CheckErr(ops.run())

		},
	}
	listEventsCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	listEventsCmd.Flags().StringVarP(&ops.since, "since", "", "", "Duration of lookup")
	listEventsCmd.Flags().BoolVarP(&ops.url, "url", "u", true, "Print cloud console URL to event")
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

	return nil
}

func ParseDurationToUTC(input string) (time.Time, error) {

	duration, err := time.ParseDuration(input)
	if err != nil {
		return time.Time{}, err
	}

	return time.Now().UTC().Add(-duration), nil
}

func GetEvents(since string, client *cloudtrail.Client, pages int) ([]*cloudtrail.LookupEventsOutput, error) {

	ctx := context.TODO()
	starttime, err := ParseDurationToUTC(since)

	if err != nil {
		return nil, err
	}
	cloudtrailClient := client
	if err != nil {
		return nil, err
	}

	Events := []*cloudtrail.LookupEventsOutput{}
	var input = cloudtrail.LookupEventsInput{
		StartTime: &starttime,
		LookupAttributes: []types.LookupAttribute{
			{AttributeKey: "ReadOnly", AttributeValue: aws.String("true")},
		},
	}

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
func ExtractUserDetails(cloudTrailEvent *string) (*RawEventDetails, error) {
	if cloudTrailEvent == nil || *cloudTrailEvent == "" {
		return &RawEventDetails{}, fmt.Errorf("cannot parse a nil input")
	}
	var res RawEventDetails
	err := json.Unmarshal([]byte(*cloudTrailEvent), &res)
	if err != nil {
		return &RawEventDetails{}, fmt.Errorf("could not marshal event.CloudTrailEvent: %w", err)
	}
	supportedEventVersions := []string{"1.08", "1.09"}
	if !slices.Contains(supportedEventVersions, res.EventVersion) {
		return &RawEventDetails{},
			fmt.Errorf("cloudtrail event version '%s' is not yet supported by cloudtrailctl",
				res.EventVersion)
	}
	return &res, nil
}
func GenerateLink(raw RawEventDetails, aws aws.Config) (url_link string) {
	str1 := "https://"
	str2 := ".console.aws.amazon.com/cloudtrailv2/home?region="
	str3 := "#/events/"
	configRegion := aws.Region
	region := raw.EventRegion
	eventId := raw.EventId

	var url = str1 + configRegion + str2 + region + str3 + eventId
	url_link = url

	return url_link
}

func PrintEvents(filteredEvents []types.Event, aws aws.Config) {

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
			fmt.Printf("User: %s |", *event.Username)
		} else {
			fmt.Println("User: <not available> |")
		}
		if event.CloudTrailEvent != nil {
			details, err := ExtractUserDetails(event.CloudTrailEvent)
			if err != nil {
				return
			}
			fmt.Printf("EventLink: %s\n", termlink.Link("click", GenerateLink(*details, aws)))

		} else {
			fmt.Println("EventLink	: <not available>")
		}

	}

}

func LoadConfiguration() (*Configuration, error) {
	var config *Configuration

	osdctlConfig.EnsureConfigFile()
	err := viper.Unmarshal(&config)
	if err != nil {
		fmt.Printf("Error Unmashaling:")
		return nil, err
	}

	return config, err

}

func FilterUsers(lookupOutputs []*cloudtrail.LookupEventsOutput) (*[]types.Event, error) {
	filteredEvents := []types.Event{}
	config, err := LoadConfiguration()
	if err != nil {
		fmt.Println("Error Loading Configuration: ")
		return nil, err
	}
	osdctlConfig.EnsureConfigFile()

	ignoreUsers := config.Ignore.Users

	for _, lookupOutput := range lookupOutputs {
		for _, event := range lookupOutput.Events {

			matched := false
			for _, regStr := range ignoreUsers {
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

func (o *LookupEventsoptions) run() error {
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

		return err
	}

	cloudtrailClient := cloudtrail.NewFromConfig(cfg)

	lookupOutput, err := GetEvents(o.since, cloudtrailClient, o.pages)

	if err != nil {
		return err
	}
	fmt.Printf("\n\n[+] Fetching %s Event History..\n", cfg.Region)
	filtered, err := FilterUsers(lookupOutput)
	PrintEvents(*filtered, cfg)

	fmt.Println("")
	return err
}
