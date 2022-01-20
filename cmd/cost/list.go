package cost

import (
	"fmt"
	"log"
	"sort"

	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
func newCmdList(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newListOptions(streams, globalOpts)
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List the cost of each Account/OU under given OU",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.checkArgs(cmd, args))
			cmdutil.CheckErr(ops.runList())
		},
	}
	listCmd.Flags().StringVar(&ops.ou, "ou", "", "get OU ID")
	// list supported time args
	listCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time. One of 'LM', 'MTD', 'YTD', '3M', '6M', '1Y'")
	listCmd.Flags().StringVar(&ops.start, "start", "", "set start date range")
	listCmd.Flags().StringVar(&ops.end, "end", "", "set end date range")
	listCmd.Flags().BoolVar(&ops.csv, "csv", false, "output result as csv")
	listCmd.Flags().StringVar(&ops.level, "level", "ou", "Cost cummulation level: possible options: ou, account")

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

	o.output = o.GlobalOptions.Output

	return nil
}

//Store flag options for get command
type listOptions struct {
	ou     string
	time   string
	start  string
	end    string
	level  string
	csv    bool
	output string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type listCostResponse struct {
	OuId    string  `json:"ouid" yaml:"ouid"`
	OuName  string  `json:"ouname" yaml:"ouname"`
	CostUSD float64 `json:"costUSD" yaml:"costUSD"`
}

func (f listCostResponse) String() string {

	return fmt.Sprintf("  OuId: %s\n  OuName: %s\n  Cost: %f\n", f.OuId, f.OuName, f.CostUSD)

}

type listAccountCostResponse struct {
	AccountId string  `json:"accountid" yaml:"accountid"`
	Unit      string  `json:"unit" yaml:"unit"`
	Cost      float64 `json:"cost" yaml:"cost"`
}

func (f listAccountCostResponse) String() string {
	return fmt.Sprintf("  AccountId: %s\n  Unit: %s\n  Cost: %f\n", f.AccountId, f.Unit, f.Cost)

}

func newListOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *listOptions {
	return &listOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (ops *listOptions) runList() error {
	awsClient, err := opsCost.initAWSClients()
	cmdutil.CheckErr(err)

	OU := getOU(awsClient, ops.ou)

	var cost float64
	var unit string
	ouCost := OUCost{
		OU:      OU,
		options: ops,
	}

	if ops.level == "ou" {
		if err := listCostsUnderOU(OU, awsClient, ops); err != nil {
			log.Fatalln("Error listing costs under OU:", err)
		}
	} else {
		ouCost.getCost(awsClient) // Get cost per account, print per account
		if ops.level == "account" {
			ouCost.printCostPerAccount()
		}
		cost, unit, err = ouCost.getSum() // Sum up account costs
		if err != nil {
			log.Fatalln("Error summing up cost of OU:", err)
		}
		printCostList(cost, unit, OU, ops, true) // TODO: Update bool here
	}

	return nil
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

	o := &getOptions{
		time:  ops.time,
		start: ops.start,
		end:   ops.end,
		ou:    ops.ou,
	}
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

type AccountCost struct {
	AccountID string
	Cost      float64
	Unit      string
}

type OUCost struct {
	Costs   []AccountCost
	OU      *organizations.OrganizationalUnit
	options *listOptions
}

func (o *OUCost) getCost(awsClient awsprovider.Client) error {

	accounts, err := getAccountsRecursive(o.OU, awsClient)
	if err != nil {
		return err
	}

	ops := &getOptions{
		time:  o.options.time,
		start: o.options.start,
		end:   o.options.end,
		ou:    o.options.ou,
	}

	for _, account := range accounts {
		accCost := AccountCost{
			AccountID: *account,
			Unit:      "",
			Cost:      0,
		}
		err = ops.getAccountCost(account, &accCost.Unit, awsClient, &accCost.Cost)
		if err != nil {
			return err
		}
		o.Costs = append(o.Costs, accCost)
	}

	sort.Slice(o.Costs, func(i, j int) bool {
		return o.Costs[i].Cost < o.Costs[j].Cost
	})

	return nil
}

func (o OUCost) getSum() (sum float64, unit string, err error) {
	sum = 0
	for _, cost := range o.Costs {
		sum += cost.Cost
		if unit == "" {
			unit = cost.Unit
			continue
		}
		if unit != cost.Unit {
			err = fmt.Errorf("can't sum up different currencies: %s and %s", unit, cost.Unit)
			return
		}
	}
	return
}

func (o OUCost) printCostPerAccount() {

	if o.options.csv {
		fmt.Println("AccountID,Cost,Unit")
	}
	for _, accountCost := range o.Costs {
		resp := listAccountCostResponse{
			AccountId: accountCost.AccountID,
			Cost:      accountCost.Cost,
			Unit:      accountCost.Unit,
		}
		if o.options.csv {
			fmt.Printf("%s,%.2f,(%s)\n", accountCost.AccountID, accountCost.Cost, accountCost.Unit)
			continue
		}
		outputflag.PrintResponse(o.options.output, resp)
	}
}

func printCostList(cost float64, unit string, OU *organizations.OrganizationalUnit, ops *listOptions, isChildNode bool) error {

	resp := listCostResponse{
		OuId:    *OU.Id,
		OuName:  *OU.Name,
		CostUSD: cost,
	}

	if !isChildNode {
		if ops.csv {
			fmt.Printf("OU,Name,Cost (%s)\n", unit)
			return nil
		} else {
			fmt.Println("Costs of OU and its child OUs:")
		}
	}

	if ops.csv {
		fmt.Printf("%v,%v,%.2f\n", *OU.Id, *OU.Name, cost)
		return nil
	}

	outputflag.PrintResponse(ops.output, resp)

	return nil
}
