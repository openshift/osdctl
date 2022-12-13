package capability

import (
	"encoding/json"
	"fmt"

	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

type addOptions struct {
	OrganizationID string
	SubscriptionID string
}

func newAddCmd() *cobra.Command {
	ops := addOptions{}
	addCmd := &cobra.Command{
		Use:   "add [capability] -g [organization ID]",
		Short: "adds a specific capability to a specific OCM organization.\nAvailable capabilities: hibernation, autoscaling, ovn, upgradeChannelChange",
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

func (o *addOptions) complete(cmd *cobra.Command, args []string) error {
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

func (o *addOptions) run(cmd *cobra.Command, capability string) error {

	// Initalize OCM connection
	ocmClient := utils.CreateConnection()
	defer func() {
		if err := ocmClient.Close(); err != nil {
			fmt.Printf("Cannot close the ocmClient (possible memory leak): %q", err)
		}
	}()

	body := CapabilityBody{Internal: true, Value: "true"}

	// Build request based on capability specified
	request := ocmClient.Post()
	if o.OrganizationID != "" {
		body.ResourceType = "organization"
		request.Path(fmt.Sprintf(organizationPath+"/%s"+labelsPath, o.OrganizationID))
	} else {
		// We know if OrgId isn't set, Subscription is
		body.ResourceType = "subscription"
		request.Path(fmt.Sprintf(subscriptionPath+"/%s"+labelsPath, o.SubscriptionID))
	}

	body.Key = getCapabilityKey(capability)

	// Now that the body is filled out, convert it to bytes
	messageBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("cannot marshal template to json: %v", err)
	}

	request.Bytes(messageBytes)

	// Post request
	response, err := request.Send()
	if err != nil {
		return fmt.Errorf("cannot send request: %q", err)
	}

	fmt.Println(response)

	return nil
}
