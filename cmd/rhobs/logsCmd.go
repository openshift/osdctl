package rhobs

import (
	"github.com/openshift/osdctl/pkg/k8s"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	clusterId string
)

func newCmdLogs() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs --cluster-id <cluster-identifier>",
		Short: "Fetch logs from RHOBS.next",
		Run: func(cmd *cobra.Command, args []string) {
			var err error
			if clusterId == "" {
				clusterId, err = k8s.GetCurrentCluster()
				if err != nil {
					cmdutil.CheckErr(err)
				}
			}

			err = main_(clusterId)
			if err != nil {
				cmdutil.CheckErr(err)
			}
		},
	}

	return cmd
}

func main_(clusterId string) error {
	return nil
}
