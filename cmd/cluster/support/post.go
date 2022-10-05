package support

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	"github.com/openshift-online/ocm-cli/pkg/dump"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/internal/support"
	"github.com/openshift/osdctl/internal/utils"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	ctlutil "github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	LimitedSupport support.LimitedSupport
	template       string
	isDryRun       bool
)

const (
	defaultTemplate = ""
)

type postOptions struct {
	output    string
	verbose   bool
	clusterID string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmdpost(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {

	ops := newPostOptions(streams, flags, globalOpts)
	postCmd := &cobra.Command{
		Use:               "post CLUSTER_ID",
		Short:             "Send limited support reason to a given cluster",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	// Define required flags
	postCmd.Flags().StringVarP(&template, "template", "t", defaultTemplate, "Message template file or URL")
	postCmd.Flags().BoolVarP(&isDryRun, "dry-run", "d", false, "Dry-run - print the limited support reason about to be sent but don't send it.")
	postCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return postCmd
}

func newPostOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *postOptions {

	return &postOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *postOptions) complete(cmd *cobra.Command, args []string) error {

	if len(args) != 1 {
		return cmdutil.UsageErrorf(cmd, "Provide exactly one internal cluster ID")
	}

	o.clusterID = args[0]
	o.output = o.GlobalOptions.Output

	return nil
}

func (o *postOptions) run() error {

	// Parse the given JSON template provided via '-t' flag
	// and load it into the LimitedSupport variable
	readTemplate()

	// Check that the cluster key (name, identifier or external identifier) given by the user
	// is reasonably safe so that there is no risk of SQL injection
	err := ctlutil.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}

	//if the cluster key is on the right format
	//create connection to sdk
	connection := ctlutil.CreateConnection()
	defer func() {
		if err := connection.Close(); err != nil {
			fmt.Printf("Cannot close the connection: %q\n", err)
			os.Exit(1)
		}
	}()

	// Print limited support template to be sent
	fmt.Printf("The following limited support reason will be sent to %s:\n", o.clusterID)
	if err := printTemplate(); err != nil {
		fmt.Printf("Cannot read generated template: %q\n", err)
		os.Exit(1)
	}

	// Stop here if dry-run
	if isDryRun {
		return nil
	}

	// ConfirmSend prompt to confirm
	err = ctlutil.ConfirmSend()
	if err != nil {
		return err
	}

	//getting the cluster
	cluster, err := ctlutil.GetCluster(connection, o.clusterID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Can't retrieve cluster: %v\n", err)
		os.Exit(1)
	}

	// postRequest calls createPostRequest and take in client and clustersmgmt/v1.cluster object
	postRequest, err := createPostRequest(connection, cluster)
	if err != nil {
		fmt.Printf("failed to create post request %q\n", err)
	}
	postResponse, err := sendRequest(postRequest)
	if err != nil {
		fmt.Printf("Failed to get post call response: %q\n", err)
	}

	// check if response matches LimitedSupport
	err = check(postResponse, LimitedSupport)
	if err != nil {
		fmt.Printf("Failed to check postResponse %q\n", err)
	}
	return nil
}

// createPostRequest create and populates the limited support post call
// swagger code gen: https://api.openshift.com/?urls.primaryName=Clusters%20management%20service#/default/post_api_clusters_mgmt_v1_clusters__cluster_id__limited_support_reasons
//SDKConnection is an interface that is satisfied by the sdk.Connection and by our mock connection
//this facilitates unit test and allow us to mock Post() and Delete() api calls
func createPostRequest(ocmClient SDKConnection, cluster *v1.Cluster) (request *sdk.Request, err error) {

	targetAPIPath := "/api/clusters_mgmt/v1/clusters/" + cluster.ID() + "/limited_support_reasons"

	request = ocmClient.Post()
	err = arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %v", targetAPIPath, err)
	}

	// pass template as `--body` of API call
	err = arguments.ApplyBodyFlag(request, template)
	if err != nil {
		return nil, fmt.Errorf("cannot apply body flag '%s'", err)
	}
	return request, nil
}

// readTemplate loads the template into the LimitedSupport variable
func readTemplate() {

	if template == defaultTemplate {
		log.Fatalf("Template file is not provided. Use '-t' to fix this.")
	}

	// check if this URL or file and if we can access it
	file, err := accessFile(template)
	if err != nil {
		log.Fatal(err)
	}

	if err = parseTemplate(file); err != nil {
		log.Fatalf("Cannot not parse the JSON template.\nError: %q\n", err)
	}
}

// accessTemplate returns the contents of a local file or url, and any errors encountered
func accessFile(filePath string) ([]byte, error) {

	// when template is file on disk
	if utils.FileExists(filePath) {
		file, err := ioutil.ReadFile(filePath) //#nosec G304 -- filePath cannot be constant
		if err != nil {
			return file, fmt.Errorf("cannot read the file.\nError: %q", err)
		}
		return file, nil
	}
	if utils.FolderExists(filePath) {
		return nil, fmt.Errorf("the provided path %q is a directory, not a file", filePath)
	}

	// when template is URL
	if utils.IsValidUrl(filePath) {
		urlPage, _ := url.Parse(filePath)
		if err := utils.IsOnline(*urlPage); err != nil {
			return nil, fmt.Errorf("host %q is not accessible", filePath)
		}
		return utils.CurlThis(urlPage.String())
	}
	return nil, fmt.Errorf("cannot read the file %q", filePath)
}

// parseTemplate reads the template file into a JSON struct
func parseTemplate(jsonFile []byte) error {
	return json.Unmarshal(jsonFile, &LimitedSupport)
}

func printTemplate() error {

	limitedSupportMessage, err := json.Marshal(LimitedSupport)
	if err != nil {
		return err
	}
	return dump.Pretty(os.Stdout, limitedSupportMessage)
}

func validateGoodResponse(body []byte, limitedSupport support.LimitedSupport) (goodReply *support.GoodReply, err error) {

	if !json.Valid(body) {
		return nil, fmt.Errorf("Server returned invalid JSON")
	}

	if err = json.Unmarshal(body, &goodReply); err != nil {
		return nil, fmt.Errorf("Cannot parse JSON template.\nError: %q", err)
	}
	return goodReply, nil
}

func validateBadResponse(body []byte) (badReply *support.BadReply, err error) {

	if ok := json.Valid(body); !ok {
		return nil, fmt.Errorf("Server returned invalid JSON")
	}
	if err = json.Unmarshal(body, &badReply); err != nil {
		return nil, fmt.Errorf("Cannot parse the error JSON meessage: %q", err)
	}
	return badReply, nil
}

func check(response *sdk.Response, limitedSupport support.LimitedSupport) error {

	body := response.Bytes()
	if response.Status() == http.StatusCreated {
		_, err := validateGoodResponse(body, limitedSupport)
		if err != nil {
			return fmt.Errorf("failed to validate good response: %q", err)
		}
		fmt.Printf("Limited support reason has been sent successfully\n")
		return nil
	}

	badReply, err := validateBadResponse(body)
	if err != nil {
		return fmt.Errorf("failed to validate bad response: %v", err)
	}
	return fmt.Errorf("bad response reason is: %s", badReply.Reason)
}
