package saas

import (
	"fmt"
	"os"

	"github.com/openshift/osdctl/cmd/promote/git"
	"github.com/spf13/cobra"
)

type saasOptions struct {
	list bool
	osd  bool
	hcp  bool

	appInterfaceCheckoutDir string
	serviceName             string
	gitHash                 string
}

// newCmdSaas implementes the saas command to interact with promoting SaaS services/operators
func NewCmdSaas() *cobra.Command {
	ops := &saasOptions{}
	saasCmd := &cobra.Command{
		Use:               "saas",
		Short:             "Utilities to promote SaaS services/operators",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Example: `
		# List all SaaS services/operators
		osdctl promote saas --list

		# Promote a SaaS service/operator
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --osd
		or
		osdctl promote saas --serviceName <service-name> --gitHash <git-hash> --hcp`,
		Run: func(cmd *cobra.Command, args []string) {
			ops.validateSaasFlow()
			appInterface := git.BootstrapOsdCtlForAppInterfaceAndServicePromotions(ops.appInterfaceCheckoutDir)

			if ops.list {
				if ops.serviceName != "" || ops.gitHash != "" || ops.osd || ops.hcp {
					fmt.Printf("Error: --list cannot be used with any other flags\n\n")
					cmd.Help()
					os.Exit(1)
				}
				listServiceNames(appInterface)
				os.Exit(0)
			}

			if !(ops.osd || ops.hcp) && ops.serviceName != "" {
				fmt.Printf("Error: --serviceName cannot be used without either --osd or --hcp\n\n")
				cmd.Help()
				os.Exit(1)
			}

			err := servicePromotion(appInterface, ops.serviceName, ops.gitHash, ops.osd, ops.hcp)
			if err != nil {
				fmt.Printf("Error while promoting service: %v\n", err)
				os.Exit(1)
			}

			os.Exit(0)

		},
	}

	saasCmd.Flags().BoolVarP(&ops.list, "list", "l", false, "List all SaaS services/operators")
	saasCmd.Flags().StringVarP(&ops.serviceName, "serviceName", "", "", "SaaS service/operator getting promoted")
	saasCmd.Flags().StringVarP(&ops.gitHash, "gitHash", "g", "", "Git hash of the SaaS service/operator commit getting promoted")
	saasCmd.Flags().BoolVarP(&ops.osd, "osd", "", false, "OSD service/operator getting promoted")
	saasCmd.Flags().BoolVarP(&ops.hcp, "hcp", "", false, "HCP service/operator getting promoted")
	saasCmd.Flags().StringVarP(&ops.appInterfaceCheckoutDir, "appInterfaceDir", "", "", "location of app-interfache checkout. Falls back to `pwd` and "+git.DefaultAppInterfaceDirectory())

	return saasCmd
}

func (o *saasOptions) validateSaasFlow() {
	if o.serviceName == "" && o.gitHash == "" {
		fmt.Printf("Usage: For SaaS services/operators, please provide --serviceName and (optional) --gitHash\n")
		fmt.Printf("--serviceName is the name of the service, i.e. saas-managed-cluster-config\n")
		fmt.Printf("--gitHash is the target git commit in the service, if not specified defaults to HEAD of master\n\n")
		return
	}
}
