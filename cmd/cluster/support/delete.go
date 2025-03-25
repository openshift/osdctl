package support

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/support"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/utils"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type deleteOptions struct {
	output                 string
	verbose                bool
	clusterID              string
	limitedSupportReasonID string
	removeAll              bool
	isDryRun               bool

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmddelete(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {

	ops := newDeleteOptions(streams, globalOpts)
	deleteCmd := &cobra.Command{
		Use:               "delete --cluster-id <cluster-identifier>",
		Short:             "Delete specified limited support reason for a given cluster",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	// Defined required flags
	deleteCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "c", "", "Internal cluster ID (required)")
	deleteCmd.Flags().BoolVar(&ops.removeAll, "all", false, "Remove all limited support reasons")
	deleteCmd.Flags().StringVarP(&ops.limitedSupportReasonID, "limited-support-reason-id", "i", "", "Limited support reason ID")
	deleteCmd.Flags().BoolVarP(&ops.isDryRun, "dry-run", "d", false, "Dry-run - print the limited support reason about to be sent but don't send it.")
	deleteCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	// Mark cluster-id as required
	deleteCmd.MarkFlagRequired("cluster-id")

	return deleteCmd
}

func newDeleteOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *deleteOptions {

	return &deleteOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *deleteOptions) complete(cmd *cobra.Command, args []string) error {

	if o.limitedSupportReasonID != "" && o.removeAll {
		return cmdutil.UsageErrorf(cmd, "Cannot provide a reason ID with the `all` flag. Please provide one or the other.")
	}

	o.output = o.GlobalOptions.Output

	return nil
}

func (o *deleteOptions) run() error {

	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	err := ctlutil.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}

	// Create an OCM client to talk to the cluster API
	connection, err := ctlutil.CreateConnection()
	if err != nil {
		return err
	}
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	// Stop here if dry-run
	if o.isDryRun {
		return nil
	}

	// confirmSend prompt to confirm
	if !utils.ConfirmPrompt() {
		return nil
	}

	//getting the cluster
	cluster, err := ctlutil.GetCluster(connection, o.clusterID)
	if err != nil {
		return fmt.Errorf("Can't retrieve cluster: %v\n", err)
	}

	/*conditions to check the presence of --all & -i flags;
	also, checking if there is one or more limited support resonID before deleting the same */
	var limitedSupportReasonIds []string
	limitedSupportReasons, err := getLimitedSupportReasons(o.clusterID)

	if len(limitedSupportReasons) == 0 {
		return fmt.Errorf("Cluster is not in limited support. \n")
	}

	if o.removeAll || (len(limitedSupportReasons) > 1 && o.limitedSupportReasonID == "") {
		if !o.removeAll && o.limitedSupportReasonID == "" {
			return fmt.Errorf("This cluster has multiple limited support reason IDs.\nPlease specify the exact reason ID or the `all` flag \n")
		}
		for _, limitedSupportReason := range limitedSupportReasons {
			err = deleteLimitedSupportReason(connection, cluster, limitedSupportReason.ID())
		}
	} else {
		if len(limitedSupportReasons) == 1 {
			o.limitedSupportReasonID = limitedSupportReasons[0].ID()
		}
		limitedSupportReasonIds = append(limitedSupportReasonIds, o.limitedSupportReasonID)
		for _, limitedSupportReasonId := range limitedSupportReasonIds {
			err = deleteLimitedSupportReason(connection, cluster, limitedSupportReasonId)
		}
	}
	return err
}

func deleteLimitedSupportReason(connection SDKConnection, cluster *v1.Cluster, reasonID string) (err error) {
	deleteRequest, err := createDeleteRequest(connection, cluster, reasonID)
	if err != nil {
		return fmt.Errorf("failed post call %q\n", err)
	}
	deleteResponse, err := utils.SendRequest(deleteRequest)
	if err != nil {
		return fmt.Errorf("Failed to get delete call response: %q\n", err)
	}

	err = checkDelete(deleteResponse)
	if err != nil {
		return fmt.Errorf("check for delete call failed: %q", err)
	}
	return nil
}

// createDeleteRequest sets the delete API and returns a request
// SDKConnection is an interface that is satisfied by the sdk.Connection and by our mock connection
// this facilitates unit test and allow us to mock Post() and Delete() api calls
func createDeleteRequest(ocmClient SDKConnection, cluster *v1.Cluster, reasonID string) (request *sdk.Request, err error) {

	targetAPIPath := "/api/clusters_mgmt/v1/clusters/" + cluster.ID() + "/limited_support_reasons/" + reasonID

	request = ocmClient.Delete()
	err = arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %v", targetAPIPath, err)
	}
	return request, nil
}

// checkDelete checks the response from delete API call
// 204 if success, otherwise error
func checkDelete(response *sdk.Response) error {

	var badReply *support.BadReply
	body := response.Bytes()
	if response.Status() == http.StatusNoContent {
		fmt.Printf("Limited support reason deleted successfully\n")
		return nil
	}

	if ok := json.Valid(body); !ok {
		return fmt.Errorf("server returned invalid JSON")
	}

	if err := json.Unmarshal(body, &badReply); err != nil {
		return fmt.Errorf("cannot parse the error JSON meessage: %q", err)
	}
	return nil
}
