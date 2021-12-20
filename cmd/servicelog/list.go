package servicelog

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	ocm_clustermgmt "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	ocm_servicelog "github.com/openshift-online/ocm-sdk-go/servicelogs/v1"

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
		ocmClient := createConnection()
		defer func() {
			if err := ocmClient.Close(); err != nil {
				log.Errorf("Cannot close the ocmClient (possible memory leak): %q", err)
			}
		}()

		// Use the OCM client to retrieve clusters
		clusters := getClusters(ocmClient, args)

		caughtErrors := make(map[string]error)
		// send it as logservice and validate the response
		for _, cluster := range clusters {
			response, err := createListRequest(ocmClient, cluster.ExternalID(), serviceLogListAllMessagesFlag).Send()
			if err != nil {
				caughtErrors[cluster.ID()] = err
				continue
			}

			entries := response.Items().Slice()
			// should sort the servicelogs (default behaviour)
			if !serviceLogListNoSort {
				sort.Slice(entries, func(i, j int) bool {
					return entries[i].Timestamp().Before(entries[j].Timestamp())
				})
			}
			if serviceLogLastEntry {
				entry := entries[len(entries)-1]
				printEntry(entry)
				continue
			}

			buf := bytes.NewBuffer(nil)
			// TODO: this hack makes formatting an array to a dictionary for parsing
			buf.WriteString("{\"entries\":")

			err = ocm_servicelog.MarshalLogEntryList(entries, buf)
			if err != nil {
				cmd.Help()
				caughtErrors[cluster.ID()] = err
				continue
			}
			buf.WriteString("}")

			err = dump.Pretty(os.Stdout, buf.Bytes())
			if err != nil {
				caughtErrors[cluster.ID()] = err
				continue
			}
		}
		if len(caughtErrors) != 0 {
			for k, v := range caughtErrors {
				fmt.Printf("error %v was caught on cluster %s\n", v, k)
			}
			return nil
		}
		return nil
	},
}

var serviceLogListAllMessagesFlag = false
var serviceLogListNoSort = false
var serviceLogLastEntry = false

func init() {
	// define required flags
	listCmd.Flags().BoolVarP(&serviceLogListAllMessagesFlag, "all-messages", "A", serviceLogListAllMessagesFlag, "Toggle if we should see all of the messages or only SRE-P specific ones")
	listCmd.Flags().BoolVarP(&serviceLogListNoSort, "no-sort", "S", serviceLogListNoSort, "Toggle if we should sort the messages by date")
	listCmd.Flags().BoolVarP(&serviceLogLastEntry, "last-entry", "l", serviceLogLastEntry, "Toggle if we should print only the last LogEntry")
}

func getClusters(ocmClient *sdk.Connection, clusterIds []string) []*ocm_clustermgmt.Cluster {
	for i, id := range clusterIds {
		clusterIds[i] = generateQuery(id)
	}

	clusters, err := applyFilters(ocmClient, []string{strings.Join(clusterIds, " or ")})
	if err != nil {
		log.Fatalf("Error while retrieving cluster(s) from ocm: %[1]s", err)
	}

	return clusters
}

func createListRequest(ocmClient *sdk.Connection, clusterId string, allMessages bool) *ocm_servicelog.ClusterLogsListRequest {
	// Create and populate the request:
	request := ocmClient.ServiceLogs().V1().ClusterLogs().List()

	formatMessage := fmt.Sprintf(`cluster_uuid = '%s'`, clusterId)
	if !allMessages {
		formatMessage += ` and service_name = 'SREManualAction'`
	}
	request = request.Search(formatMessage)

	return request
}
