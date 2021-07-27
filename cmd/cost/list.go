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
			cmdutil.CheckErr(ops.checkArgs(cmd, args))
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
	listCmd.Flags().StringVar(&ops.start, "start", "", "set start date range")
	listCmd.Flags().StringVar(&ops.end, "end", "", "set end date range")
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

func (o *listOptions) checkArgs(cmd *cobra.Command, _ []string) error {
	// check that only time or start/end is provided
	if o.start == "" && o.end == "" && o.time == "" {
		return cmdutil.UsageErrorf(cmd, "Please provide a date range or a predefined time")
	}
	if o.start != "" && o.end != "" && o.time != "" {
		return cmdutil.UsageErrorf(cmd, "Please provide either a date range or a predefined time")
	}
	if o.start != "" && o.end == "" {
		return cmdutil.UsageErrorf(cmd, "Please provide end of date range")
	}
	if o.start == "" && o.end != "" {
		return cmdutil.UsageErrorf(cmd, "Please provide start of date range")
	}
	if o.ou == "" {
		return cmdutil.UsageErrorf(cmd, "Please provide OU")
	}
	return nil
}

//Store flag options for get command
type listOptions struct {
	ou    string
	time  string
	start string
	end   string
	csv   bool

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

	o := &getOptions{}
	if err := o.getOUCostRecursive(&cost, &unit, OU, awsClient); err != nil {
		return err
	}

	//Print cost of given OU
	printCostList(cost, unit, OU, ops, isChildNode)

	//Print costs of child OUs under given OU
	for _, childOU := range OUs {
		cost = 0
		isChildNode = true

		if err := o.getOUCostRecursive(&cost, &unit, childOU, awsClient); err != nil {
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
