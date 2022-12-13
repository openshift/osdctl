package capability

import (
	"fmt"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type removeOptions struct {
	OrganizationID string
	SubscriptionID string
}

func newRemoveCmd() *cobra.Command {
	ops := removeOptions{}
	addCmd := &cobra.Command{
		Use:   "remove [capability] -g [organization ID]",
		Short: "Removes a specific capability to a specific OCM organization.\nAvailable capabilities: hibernation, autoscaling, ovn, upgradeChannelChange",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(cmd, args[0]))
		},
		Deprecated: "This command is being deprecated in lieu of using git-backed capabilities. Soon, this command will not work, and you will have to follow the SOP at https://github.com/openshift/ops-sop/v4/howto/capabilities.md.",
	}

	addCmd.Flags().StringVarP(&ops.OrganizationID, "organization-id", "g", "", "Specify an OCM Organization to apply a capability to")
	addCmd.Flags().StringVarP(&ops.SubscriptionID, "subscription-id", "b", "", "Specify a Subscription to apply a capability to")

	return addCmd
}

func (o *removeOptions) complete(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("capability was not provided, please specifiy which capability to add")
	}

	if len(args) != 1 {
		return fmt.Errorf("too many arguments. Expected 1 got %d", len(args))
	}

	// Ensure that the capability selected is valid
	availableCapabilities := []string{hibernation, autoscaling, ovn, upgradeChannelChange}
	for i, s := range availableCapabilities {
		if args[0] == s {
			break
		}
		if i == len(availableCapabilities)-1 {
			return fmt.Errorf("invalid capability specified. Available capabilites are hibernation, autoscaling, ovn and upgradeChannelChange")
		}
	}

	// Make sure only of of Org ID or Sub ID is set
	if o.OrganizationID != "" && o.SubscriptionID != "" {
		return fmt.Errorf("organization and subscription cannot both be set. Please only specify one")
	}

	// Make sure at least one of them is set
	if o.OrganizationID == "" && o.SubscriptionID == "" {
		return fmt.Errorf("no organization or subscription was provided, please specify organization using -g, or subscription using -b")
	}

	return nil
}

func (o *removeOptions) run(cmd *cobra.Command, capability string) error {

	// Initalize OCM connection
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	// Build the base of the request path
	var base string
	if o.OrganizationID != "" {
		// If the organizationID is set
		base = organizationPath + "/" + o.OrganizationID
	} else {
		// If organization ID isn't spcified, we know subscription ID is
		base = subscriptionPath + "/" + o.SubscriptionID
	}

	// Build href path for capability
	href := fmt.Sprintf(base + labelsPath + "/" + getCapabilityKey(capability))

	// Create a deletion request and add the href as the path
	request := ocmClient.Delete()
	request.Path(href)

	// Send the request
	response, err := request.Send()
	if err != nil {
		fmt.Println(response)
		return fmt.Errorf("cannot send request: %q", err)
	}

	return nil
}
