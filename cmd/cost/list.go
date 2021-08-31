package cost

import (
	"fmt"
	"log"

	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
func newCmdList(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newListOptions(streams)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List the cost of each OU under given OU",
		Run: func(cmd *cobra.Command, args []string) {

			awsClient, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			OU := getOU(awsClient, ops.ou)

			if err := listCostsUnderOU(OU, awsClient, ops); err != nil {
				log.Fatalln("Error listing costs under OU:", err)
			}
		},
	}
	listCmd.Flags().StringVar(&ops.ou, "ou", "", "get OU ID")
	// list supported time args
	listCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time. One of 'LM', 'MTD', 'TYD', '3M', '6M', '1Y'")
	listCmd.Flags().BoolVar(&ops.csv, "csv", false, "output result as csv")

	if err := listCmd.MarkFlagRequired("ou"); err != nil {
		log.Fatalln("OU flag:", err)
	}
	// require explicit time set
	if err := listCmd.MarkFlagRequired("time"); err != nil {
		log.Fatalln("time flag:", err)
	}

	return listCmd
}

//Store flag options for get command
type listOptions struct {
	ou   string
	time string
	csv  bool

	genericclioptions.IOStreams
}

func newListOptions(streams genericclioptions.IOStreams) *listOptions {
	return &listOptions{
		IOStreams: streams,
	}
}

//List the cost of each OU under given OU
func listCostsUnderOU(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client, ops *listOptions) error {
	OUs, err := getOUsRecursive(OU, awsClient)
	if err != nil {
		return err
	}

	var cost float64
	var unit string
	var isChildNode bool

	if err := getOUCostRecursive(&cost, &unit, OU, awsClient, &ops.time); err != nil {
		return err
	}

	//Print cost of given OU
	printCostList(cost, unit, OU, ops, isChildNode)

	//Print costs of child OUs under given OU
	for _, childOU := range OUs {
		cost = 0
		isChildNode = true

		if err := getOUCostRecursive(&cost, &unit, childOU, awsClient, &ops.time); err != nil {
			return err
		}
		printCostList(cost, unit, childOU, ops, isChildNode)
	}

	return nil
}

func printCostList(cost float64, unit string, OU *organizations.OrganizationalUnit, ops *listOptions, isChildNode bool) {
	if !isChildNode {
		if ops.csv {
			fmt.Printf("OU,Name,Cost (%s)\n", unit)
		} else {
			fmt.Printf("Costs of OU %s '%s' and its child OUs:\n\n", *OU.Id, *OU.Name)
			fmt.Printf("%-20s%-30s%-20s\n", "OU ID", "OU Name", "Cost")
		}
	}

	if ops.csv {
		fmt.Printf("%v,%v,%.2f\n", *OU.Id, *OU.Name, cost)
	} else {
		fmt.Printf("%-20s%-30s%.2f\n", *OU.Id, *OU.Name, cost)
	}
}
