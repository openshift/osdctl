package cost

import (
	"fmt"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// costCmd represents the cost command
func NewCmdCost(streams genericclioptions.IOStreams) *cobra.Command {
	costCmd := &cobra.Command{
		Use:   "cost",
		Short: "Cost Management related utilities",
		Long: `The cost command allows for cost management on the AWS platform (other 
platforms may be added in the future)`,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("cost called")
		},
	}

	costCmd.AddCommand(awsCmd)

	return costCmd
}


