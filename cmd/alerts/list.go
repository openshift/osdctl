//osdctl alerts list ${CLUSTERID} --level [warning, critical, firing, pending, all] --active bool 
package alerts

import (
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"log"
)

func NewCmdList() *cobra.Command {
	return &cobra.Command{
		Use:               "list <cluster-id>",
		Short:             "list alerts",
		Long:              `Checks the alerts for the cluster`,
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
				cmdutil.CheckErr(ListCheck(args[0]))
			},
	}
}

func ListCheck(clusterId string) error {
	defer func() {
		if err := recover(); err != nil {
			log.Fatal("error : ", err)
		}
	}()

	/*_ := func (clusterId)
	if err != nil {
		return err
	}*/
	return nil
}