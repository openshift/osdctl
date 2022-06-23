package servicelog

import (
	"fmt"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/utils"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:           "list [flags] [options] cluster-identifier",
	Short:         "gets all servicelog messages for a given cluster",
	Args:          cobra.ArbitraryArgs,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			cmd.Help()
			return fmt.Errorf("cluster-identifier was not provided. please provide a cluster id, UUID, or name")
		}

		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := utils.CreateConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		if len(args) != 1 {
			log.Infof("Too many arguments. Expected 1 got %d", len(args))
		}

		// Use the OCM client to retrieve clusters
		clusters := utils.GetClusters(ocmClient, args)

		// send it as logservice and validate the response
		for _, cluster := range clusters {
			response, err := sendRequest(createListRequest(ocmClient, cluster.ExternalID(), serviceLogListAllMessagesFlag))
			if err != nil {
				return err
			}

			err = dump.Pretty(os.Stdout, response.Bytes())
			if err != nil {
				cmd.Help()
				return err
			}
		}
		return nil
	},
}

var serviceLogListAllMessagesFlag = false

const listServiceLogAPIPath = "/api/service_logs/v1/clusters/%s/cluster_logs"

func init() {
	// define required flags
	listCmd.Flags().BoolVarP(&serviceLogListAllMessagesFlag, "all-messages", "A", serviceLogListAllMessagesFlag, "Toggle if we should see all of the messages or only SRE-P specific ones")
}

func createListRequest(ocmClient *sdk.Connection, clusterId string, allMessages bool) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)
	}
	var empty []string

	formatMessage := fmt.Sprintf(`search=cluster_uuid = '%s'`, clusterId)
	if !allMessages {
		formatMessage += ` and service_name = 'SREManualAction'`
	}
	arguments.ApplyParameterFlag(request, []string{formatMessage})
	arguments.ApplyHeaderFlag(request, empty)
	return request
}
