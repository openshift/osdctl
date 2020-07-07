package cost

import (
	"fmt"
	"github.com/spf13/cobra"
)

// costCmd represents the cost command
var CostCmd = &cobra.Command{
	Use:   "cost",
	Short: "Cost Management related utilities",
	Long: `The cost command allows for cost management on the AWS platform (other
platforms may be added in the future. Its functions include:

- Managing the AWS Cost Explorer with $ osdctl cost aws. This leaves the possibility of adding cost 
management support for other platforms e.g. $ osdctl cost gcp

- Get cost of OUs with $ osdctl cost aws get

- Create cost category with $ osdctl cost aws create

- Reconcile cost categories with $ osdctl cost aws reconcile`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cost called")
	},
}

func init() {
	CostCmd.AddCommand(awsCmd)
}
