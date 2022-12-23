package servicelog

import (
	"fmt"
	"log"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
)

type List struct {
	AllMessages bool
	Internal    bool
}

func NewCmdList() *cobra.Command {
	l := &List{}

	cmd := &cobra.Command{
		Short: "gets all servicelog messages for a given cluster",
		Use:   "list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return l.run(cmd, args[0])
		},
	}

	cmd.Flags().BoolVarP(&l.AllMessages, "all-messages", "A", false, "Toggle if we should see all of the messages instead of only SRE-P specific ones")
	cmd.Flags().BoolVarP(&l.Internal, "internal", "i", false, "Toggle if we should see internal messages")

	return cmd
}

func (l *List) run(cmd *cobra.Command, clusterID string) error {
	response, err := l.FetchServiceLogs(clusterID)
	if err != nil {
		log.Printf("failed to fetch service logs: %s", err)
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

func (l *List) FetchServiceLogs(clusterID string) (*sdk.Response, error) {
	// Create OCM client to talk to cluster API
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			log.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Use the OCM client to retrieve clusters
	clusters := utils.GetClusters(ocmClient, []string{clusterID})
	if len(clusters) != 1 {
		return nil, fmt.Errorf("expected 1 cluster from GetClusters %s, got: %d", clusterID, len(clusters))
	}
	cluster := clusters[0]

	// Now get the SLs for the cluster
	return sendRequest(l.CreateListSLRequest(ocmClient, cluster.ExternalID()))
}

func (l *List) CreateListSLRequest(ocmClient *sdk.Connection, clusterId string) *sdk.Request {
	// Create and populate the request:
	request := ocmClient.Get()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		log.Fatalf("Can't parse API path '%s': %v\n", targetAPIPath, err)
	}

	formatMessage := fmt.Sprintf(`search=cluster_uuid = '%s'`, clusterId)
	if !l.AllMessages {
		formatMessage += ` and service_name = 'SREManualAction'`
	}
	if l.Internal {
		formatMessage += ` and internal_only = 'true'`
	}
	arguments.ApplyParameterFlag(request, []string{formatMessage})
	return request
}
