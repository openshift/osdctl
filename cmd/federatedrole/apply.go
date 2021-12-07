package federatedrole

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/openshift/osdctl/pkg/k8s"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
)

const (
	awsAccountIDLabel = "awsAccountID"
	uidLabel          = "uid"
)

// newCmdApply implements the apply command to apply federated role CR
func newCmdApply(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newApplyOptions(streams, flags, client)
	applyCmd := &cobra.Command{
		Use:               "apply",
		Short:             "Apply federated role CR",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	applyCmd.Flags().StringVarP(&ops.url, "url", "u", "", "The URL of federated role yaml file")
	applyCmd.Flags().StringVarP(&ops.file, "file", "f", "", "The path of federated role yaml file")
	applyCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")

	return applyCmd
}

// applyOptions defines the struct for running list account command
type applyOptions struct {
	url  string
	file string

	verbose bool

	flags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newApplyOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *applyOptions {
	return &applyOptions{
		flags:     flags,
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *applyOptions) complete(cmd *cobra.Command, _ []string) error {
	if o.file == "" && o.url == "" {
		return cmdutil.UsageErrorf(cmd, "Flags file and url cannot be empty at the same time")
	}

	if o.file != "" && o.url != "" {
		return cmdutil.UsageErrorf(cmd, "Flags file and url cannot be set at the same time")
	}

	return nil
}

func (o *applyOptions) run() error {
	ctx := context.TODO()

	var (
		input     io.Reader
		accountID string
		uid       string
		ok        bool
	)
	if o.url != "" {
		resp, err := http.Get(o.url)
		if err != nil {
			return err
		}

		if resp.StatusCode/100 != 2 {
			return errors.New(fmt.Sprintf("Failed to GET %s, status code %d", o.url, resp.StatusCode))
		}

		defer resp.Body.Close()
		input = resp.Body

	} else {
		path, err := filepath.Abs(o.file)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		input = file
	}

	var federatedRole awsv1alpha1.AWSFederatedRole
	d := yaml.NewYAMLOrJSONDecoder(input, 4096)
	if err := d.Decode(&federatedRole); err != nil {
		return err
	}

	// apply federated role CR yaml
	desired := federatedRole.DeepCopy()
	if _, err := controllerutil.CreateOrUpdate(ctx, o.kubeCli, &federatedRole, func() error {
		federatedRole.Spec = desired.Spec
		return nil
	}); err != nil {
		return err
	}

	var federatedAccountAccesses awsv1alpha1.AWSFederatedAccountAccessList
	if err := o.kubeCli.List(ctx, &federatedAccountAccesses, &client.ListOptions{}); err != nil {
		return err
	}

	if len(federatedAccountAccesses.Items) == 0 {
		fmt.Fprintln(o.Out, "Cannot find associated AWS federated account accesses")
		return nil
	}

	awsClients := make(map[string]awsprovider.Client, 0)

	// find all associated federated account access CR and update them
	for _, federatedAccount := range federatedAccountAccesses.Items {
		if federatedAccount.Spec.AWSFederatedRole.Namespace == federatedRole.Namespace &&
			federatedAccount.Spec.AWSFederatedRole.Name == federatedRole.Name {
			if accountID, ok = federatedAccount.Labels[awsAccountIDLabel]; !ok {
				return errors.New(fmt.Sprintf(
					"Unable to get AWS AccountID label for AWS federated account access CR %s/%s",
					federatedAccount.Namespace, federatedAccount.Name))
			}

			if uid, ok = federatedAccount.Labels[uidLabel]; !ok {
				return errors.New(fmt.Sprintf(
					"Unable to get UID label for AWS federated account access CR %s/%s",
					federatedAccount.Namespace, federatedAccount.Name))
			}

			var awsClient awsprovider.Client
			if awsClient, ok = awsClients[federatedAccount.Namespace]; !ok {
				creds, err := k8s.GetAWSAccountCredentials(ctx, o.kubeCli,
					federatedAccount.Spec.AWSCustomerCredentialSecret.Namespace,
					federatedAccount.Spec.AWSCustomerCredentialSecret.Name)
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf(
						"Failed to get AWS credentials for AWS federated account access CR %s/%s",
						federatedAccount.Namespace, federatedAccount.Name))
				}

				awsClient, err = awsprovider.NewAwsClientWithInput(creds)
				if err != nil {
					return errors.Wrap(err, fmt.Sprintf(
						"Failed to create AWS Setup Client for AWS federated account access CR %s/%s",
						federatedAccount.Namespace, federatedAccount.Name))
				}
				awsClients[federatedAccount.Namespace] = awsClient
			}

			fmt.Fprintln(o.Out, fmt.Sprintf("Updating IAM policy for AWS federated account access CR %s/%s",
				federatedAccount.Namespace, federatedAccount.Name))

			if err := awsprovider.RefreshIAMPolicy(awsClient, &federatedRole, accountID, uid); err != nil {
				return errors.Wrap(err, fmt.Sprintf("Failed to apply IAM policy for AWS federated account access CR %s/%s",
					federatedAccount.Namespace, federatedAccount.Name))
			}
		}
	}

	return nil
}
