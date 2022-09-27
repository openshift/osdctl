package cluster

import (
	"bytes"
	"fmt"
	"html/template"
	"os"

	v1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// ownerOptions defines the struct for the current command
// This command requires the ocm API Token https://cloud.redhat.com/openshift/token be available in the OCM_TOKEN env variable.
type ownerOptions struct {
	output   string
	verbose  bool
	userName string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

// newCmdOwner return a new command
func newCmdOwner(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newOwnerOptions(streams, flags, globalOpts)
	ownerCmd := &cobra.Command{
		Use:               "owner",
		Short:             "List the clusters owned by the user (can be specified to any user, not only yourself)",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	ownerCmd.Flags().StringVarP(&ops.userName, "user-id", "u", ops.userName, "user to check the cluster owner on")

	return ownerCmd
}

func newOwnerOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *ownerOptions {
	return &ownerOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *ownerOptions) complete(cmd *cobra.Command, _ []string) error {

	o.output = o.GlobalOptions.Output

	return nil
}

func (o *ownerOptions) run() error {
	connection := utils.CreateConnection()

	var (
		accountName = o.userName
		accountID   = ""
	)

	if accountName == "" {
		fmt.Println("using the current user")
		response, err := connection.AccountsMgmt().V1().CurrentAccount().Get().
			Send()
		if err != nil {
			return fmt.Errorf("Can't send request: %v", err)
		}
		accountName = response.Body().Username()
		accountID = response.Body().ID()
	} else {
		const usernameQuery = "id = '{{.}}' or username like '%{{.}}%' or email like '%{{.}}%'"
		tmplateFormat, err := template.New("").Parse(usernameQuery)
		if err != nil {
			return fmt.Errorf("could not parse template: %w", err)
		}
		var filledUpUsernameQuery bytes.Buffer

		err = tmplateFormat.Execute(&filledUpUsernameQuery, accountName)
		if err != nil {
			return fmt.Errorf("could not execute the template with the input %v: %w", accountName, err)
		}
		searchString := filledUpUsernameQuery.String()
		if o.verbose {
			fmt.Printf("the search query is '%s'\n", searchString)
		}

		response, err := connection.AccountsMgmt().V1().Accounts().List().Parameter("search", searchString).
			Send()

		if err != nil {
			return fmt.Errorf("Can't send request: %v", err)
		}

		if response.Total() != 1 {
			fmt.Println("Found users:")
			err := v1.MarshalAccountList(response.Items().Slice(), os.Stdout)
			if err != nil {
				fmt.Println("error while marshalling account list: ", err.Error())
				return err
			}
			// newline is required as MarshalAccountList doesn't enter a newline once the object is written down
			fmt.Println()
			return fmt.Errorf("given username '%s' is not unique, found '%d' matches", accountName, response.Total())
		}
		accountID = response.Items().Get(0).ID()

	}

	if accountID == "null" || accountID == "" {
		return fmt.Errorf("could not extract the accountID")
	}

	fmt.Printf("the user is '%s' with ID '%s'\n", accountName, accountID)

	const subscriptionQuery = "creator.id = '%s' and status != 'Deprovisioned' and status != 'Archived'"
	searchString := fmt.Sprintf(subscriptionQuery, accountID)
	response, err := connection.AccountsMgmt().V1().Subscriptions().List().Parameter("search", searchString).
		Send()

	if o.verbose {
		fmt.Printf("the search query is '%s'\n", searchString)
	}

	if err != nil {
		return fmt.Errorf("Can't send request: %v", err)
	}

	if response.Total() == 0 {
		return nil
	}

	fmt.Printf("'User %s owns the following clusters (total %d):\n", accountName, response.Total())

	for _, i := range response.Items().Slice() {
		fmt.Println(i.ExternalClusterID())
	}

	return nil
}
