package support

import (
	"encoding/json"
	"fmt"
	"log"
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

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmddelete(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {

	ops := newDeleteOptions(streams, flags, globalOpts)
	deleteCmd := &cobra.Command{
		Use:               "delete CLUSTER_ID",
		Short:             "Delete specified limited support reason for a given cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	// Defined required flags
	deleteCmd.Flags().StringVarP(&ops.limitedSupportReasonID, "limited-support-reason-id", "i", "", "Limited support reason ID")
	deleteCmd.Flags().BoolVarP(&isDryRun, "dry-run", "d", false, "Dry-run - print the limited support reason about to be sent but don't send it.")
	deleteCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	// Mark limited-support-reason-id (-i) flag required
	if err := deleteCmd.MarkFlagRequired("limited-support-reason-id"); err != nil {
		log.Fatalln("limited-support-reason-id", err)
	}

	return deleteCmd
}

func newDeleteOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *deleteOptions {

	return &deleteOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *deleteOptions) complete(cmd *cobra.Command, args []string) error {

	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Provide exactly one internal cluster ID")
	}

	o.clusterID = args[0]
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
	connection := ctlutil.CreateConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	// Stop here if dry-run
	if isDryRun {
		return nil
	}

	// confirmSend prompt to confirm
	err = utils.ConfirmSend()
	if err != nil {
		return err
	}

	//getting the cluster
	cluster, err := ctlutil.GetCluster(connection, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster: %v\n", err)
		os.Exit(1)
	}

	deleteRequest, err := createDeleteRequest(connection, cluster, o.limitedSupportReasonID)
	if err != nil {
		fmt.Printf("failed post call %q\n", err)
	}
	deleteResponse, err := sendRequest(deleteRequest)
	if err != nil {
		fmt.Printf("Failed to get delete call response: %q\n", err)
	}

	err = checkDelete(deleteResponse)
	if err != nil {
		fmt.Printf("check for delete call failed: %q", err)
	}

	return nil
}

// createDeleteRequest sets the delete API and returns a request
//SDKConnection is an interface that is satisfied by the sdk.Connection and by our mock connection
//this facilitates unit test and allow us to mock Post() and Delete() api calls
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
