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
platforms may be added in the future)`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("cost called")
	},
}

func init() {
	CostCmd.AddCommand(awsCmd)
}
