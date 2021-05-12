package servicelog

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/internal/servicelog"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "gets all servicelog messages for a given cluster",
	// validate only clusterid is provided
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clusterId := args[0]

		// Create an OCM client to talk to the cluster API
		// the user has to be logged in (e.g. 'ocm login')
		ocmClient := createConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		// Use the OCM client to create the POST request
		request := createClusterRequest(ocmClient, clusterId)
		response := sendRequest(request)
		clusterExternalId, err := extractExternalIdFromResponse(response)
		if err != nil {
			return err
		}

		// send it as logservice and validate the response
		request = createListRequest(ocmClient, clusterExternalId, serviceLogListAllMessagesFlag)
		response = sendRequest(request)

		err = dump.Pretty(os.Stdout, response.Bytes())
		if err != nil {
			return err
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

func extractExternalIdFromResponse(response *sdk.Response) (string, error) {
	status := response.Status()
	body := response.Bytes()

	if status >= 400 {
		validateBadResponse(body)
		log.Fatalf("Failed to list message because of %q", BadReply.Reason)
		return "", nil
	}

	validateGoodResponse(body)
	clusterListGoodReply := servicelog.ClusterListGoodReply{}
	err := json.Unmarshal(body, &clusterListGoodReply)
	if err != nil {
		err = fmt.Errorf("cannot parse good clusterlist response: %w", err)
		return "", err
	}

	if clusterListGoodReply.Total != 1 || len(clusterListGoodReply.Items) != 1 {
		return "", fmt.Errorf("could not find an exact match for the clustername")
	}

	return clusterListGoodReply.Items[0].ExternalID, nil
}

func createClusterRequest(ocmClient *sdk.Connection, clusterId string) *sdk.Request {

	searchString := fmt.Sprintf(`search=display_name like '%[1]s' or name like '%[1]s' or id like '%[1]s' or external_id like '%[1]s'`, clusterId)
	searchString = strings.TrimSpace(searchString)
	request := ocmClient.Get()
	err := arguments.ApplyPathArg(request, "/api/clusters_mgmt/v1/clusters/")
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)
	}

	arguments.ApplyParameterFlag(request, []string{searchString})

	return request
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
