package capability

import (
	"fmt"

	"github.com/spf13/cobra"
)

const (
	// All the base paths for OCM resources used for capabilities
	basePath         = "/api/accounts_mgmt/v1"
	labelsPath       = "/labels"
	organizationPath = basePath + "/organizations"
	subscriptionPath = basePath + "/subscriptions"

	hibernation          = "hibernation"
	autoscaling          = "autoscaling"
	upgradeChannelChange = "upgradeChannelChange"
	ovn                  = "ovn"

	// Source for all capabilitiy keys. Add new keys here when adding a capability to the command
	hibernationKey          = "capability.organization.hibernate_cluster"
	autoscalingKey          = "capability.cluster.autoscale_clusters"
	upgradeChannelChangeKey = "capability.organization.allow_set_upgrade_channel_group"
	ovnKey                  = "capability.organization.ovn_cluster"
)

type CapabilityBody struct {
	Internal     bool   `json:"internal"`
	Key          string `json:"key"`
	ResourceType string `json:"type"`
	Value        string `json:"value"`
}

func NewCmdCapability() *cobra.Command {
	var capabilityCmd = &cobra.Command{
		Use:   "capability",
		Short: "Manage capabilites for OCM Organizations",
		Run: func(cmd *cobra.Command, args []string) {
			err := cmd.Help()
			if err != nil {
				fmt.Println("Error calling cmd.Help(): ", err.Error())
				return
			}
		},
		Deprecated: "This command is being deprecated in lieu of using git-backed capabilities. Soon, this command will not work, and you will have to follow the SOP at https://github.com/openshift/ops-sop/v4/howto/capabilities.md.",
	}

	// Add subcommands
	capabilityCmd.AddCommand(newAddCmd())
	capabilityCmd.AddCommand(newRemoveCmd())

	return capabilityCmd
}

// Ideally, this function would instead be a map[string]string
// However, maps cannot be initalized at the package scope so
// to use a map, one would need to create a function that builds
// the desired structure, i.e. getCapabilityMap() map[string]string
// This function recreates the functionality of a map without the overhead
// of needing to build the map every time its needed.
// The use of constants prevents cases of mispellings
func getCapabilityKey(capability string) string {

	switch capability {
	case hibernation:
		return hibernationKey
	case autoscaling:
		return autoscalingKey
	case ovn:
		return ovnKey
	case upgradeChannelChange:
		return upgradeChannelChangeKey
	}

	return "Not a valid capability"
}
