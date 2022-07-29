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
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:           "list [flags] [options] cluster-identifier",
	Short:         "gets all servicelog messages for a given cluster",
	Args:          cobra.ArbitraryArgs,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		cmdutil.CheckErr(complete(cmd, args))
		cmdutil.CheckErr(run(cmd, args[0]))
	},
}

func complete(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		err := cmd.Help()
		if err != nil {
			return fmt.Errorf("error calling cmd.Help(): %w", err)

		}
		return fmt.Errorf("cluster-identifier was not provided. please provide a cluster id, UUID, or name")
	}

	if len(args) != 1 {
		log.Infof("Too many arguments. Expected 1 got %d", len(args))
	}

	return nil
}

func run(cmd *cobra.Command, clusterID string) error {
	response, err := FetchServiceLogs(clusterID)
	if err != nil {
		// If the response has errored, likely the input was bad, so show usage
		err := cmd.Help()
		if err != nil {
			return err
		}
		return err
	}

	err = dump.Pretty(os.Stdout, response.Bytes())
	if err != nil {
		// If outputing the data errored, there's likely an internal error, so just return the error
		return err
	}
	return nil
}

var serviceLogListAllMessagesFlag = false
var serviceLogListInternalOnlyFlag = false

func FetchServiceLogs(clusterID string) (*sdk.Response, error) {
	// Create OCM client to talk to cluster API
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Use the OCM client to retrieve clusters
	clusters := utils.GetClusters(ocmClient, []string{clusterID})
	if len(clusters) != 1 {
		return nil, fmt.Errorf("GetClusters expected to return 1 cluster, got: %d", len(clusters))
	}
	cluster := clusters[0]

	serviceLogListAllMessagesFlag := false

	// Now get the SLs for the cluster
	return sendRequest(CreateListSLRequest(ocmClient, cluster.ExternalID(), serviceLogListAllMessagesFlag, serviceLogListInternalOnlyFlag))
}

func init() {
	// define required flags
	listCmd.Flags().BoolVarP(&serviceLogListAllMessagesFlag, "all-messages", "A", serviceLogListAllMessagesFlag, "Toggle if we should see all of the messages or only SRE-P specific ones")
	listCmd.Flags().BoolVarP(&serviceLogListInternalOnlyFlag, "internal", "i", serviceLogListInternalOnlyFlag, "Toggle if we should see internal messages")
}

func CreateListSLRequest(ocmClient *sdk.Connection, clusterId string, allMessages bool, internalMessages bool) *sdk.Request {
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
	if internalMessages {
		formatMessage += ` and internal_only = 'true'`
	}
	arguments.ApplyParameterFlag(request, []string{formatMessage})
	arguments.ApplyHeaderFlag(request, empty)
	return request
}
