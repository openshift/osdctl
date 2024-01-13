package cost

import (
	"fmt"
	"log"
	"sort"

	"github.com/aws/aws-sdk-go-v2/service/organizations/types"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
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
	listCmd.Flags().StringArrayVar(&ops.ou, "ou", []string{}, "get OU ID")
	// list supported time args
	listCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time. One of 'LM', 'MTD', 'YTD', '3M', '6M', '1Y'")
	listCmd.Flags().StringVar(&ops.start, "start", "", "set start date range")
	listCmd.Flags().StringVar(&ops.end, "end", "", "set end date range")
	listCmd.Flags().BoolVar(&ops.csv, "csv", false, "output result as csv")
	listCmd.Flags().StringVar(&ops.level, "level", "ou", "Cost cummulation level: possible options: ou, account")
	listCmd.Flags().BoolVar(&ops.sum, "sum", true, "Hide sum rows")

	if err := listCmd.MarkFlagRequired("ou"); err != nil {
		log.Fatalln("OU flag:", err)
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
	if len(o.ou) == 0 {
		return cmdutil.UsageErrorf(cmd, "Please provide OU")
	}

	o.output = o.GlobalOptions.Output

	return nil
}

// Store flag options for get command
type listOptions struct {
	ou     []string
	time   string
	start  string
	end    string
	level  string
	csv    bool
	sum    bool
	output string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type listCostResponse struct {
	OuId    string          `json:"ouid" yaml:"ouid"`
	OuName  string          `json:"ouname" yaml:"ouname"`
	CostUSD decimal.Decimal `json:"costUSD" yaml:"costUSD"`
}

func (f listCostResponse) String() string {

	return fmt.Sprintf("  OuId: %s\n  OuName: %s\n  Cost: %s\n", f.OuId, f.OuName, f.CostUSD)

}

type listAccountCostResponse struct {
	OU        string          `json:"ou" yaml:"ou"`
	AccountId string          `json:"accountid" yaml:"accountid"`
	Unit      string          `json:"unit" yaml:"unit"`
	Cost      decimal.Decimal `json:"cost" yaml:"cost"`
}

func (f listAccountCostResponse) String() string {
	return fmt.Sprintf("  AccountId: %s\n  Unit: %s\n  Cost: %s\n", f.AccountId, f.Unit, f.Cost)

}

func newListOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *listOptions {
	return &listOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *listOptions) runList() error {
	awsClient, err := opsCost.initAWSClients()
	cmdutil.CheckErr(err)

	printHeader(o)

	for _, ou := range o.ou {
		OU := getOU(awsClient, ou)

		var cost decimal.Decimal
		var unit string

		if o.level == "ou" {
			if err := listCostsUnderOU(OU, awsClient, o); err != nil {
				log.Fatalln("Error listing costs under OU:", err)
			}
			printCostList(cost, unit, OU, o, true) // TODO: Update bool here
			return nil
		}

		if o.level == "account" {
			ouCost := OUCost{
				OU:      OU,
				options: o,
			}
			ouCost.printCostPerAccount(awsClient) // Get cost per account, print per account
		}
	}

	return nil
}

func printHeader(ops *listOptions) {
	switch ops.level {

	case "account":
		if ops.csv {
			fmt.Println("OU, AccountID,Cost,Unit")
		}
	case "ou":
		if ops.csv {
			fmt.Printf("OU,Name,Cost,Unit\n")
			break
		}
	}
}

// List the cost of each OU under given OU
func listCostsUnderOU(OU *types.OrganizationalUnit, awsClient awsprovider.Client, ops *listOptions) error {
	OUs, err := getOUsRecursive(OU, awsClient)
	if err != nil {
		return err
	}

	var cost decimal.Decimal
	var unit string
	var isChildNode bool

	o := &getOptions{
		time:  ops.time,
		start: ops.start,
		end:   ops.end,
		ou:    *OU.Id,
	}
	if err := o.getOUCostRecursive(&cost, &unit, OU, awsClient); err != nil {
		return err
	}

	//Print cost of given OU
	printCostList(cost, unit, OU, ops, isChildNode)

	//Print costs of child OUs under given OU
	for _, childOU := range OUs {
		cost = decimal.Zero
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
	Cost      decimal.Decimal
	Unit      string
}

type OUCost struct {
	Costs   []AccountCost
	OU      *types.OrganizationalUnit
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
		ou:    *o.OU.Id,
	}

	for _, account := range accounts {
		accCost := AccountCost{
			AccountID: *account,
			Unit:      "",
			Cost:      decimal.Zero,
		}
		err = ops.getAccountCost(account, &accCost.Unit, awsClient, &accCost.Cost)
		if err != nil {
			return err
		}
		o.Costs = append(o.Costs, accCost)
	}

	sort.Slice(o.Costs, func(i, j int) bool {
		return o.Costs[j].Cost.LessThan(o.Costs[i].Cost)
	})

	return nil
}

func (o *OUCost) getSum() (sum decimal.Decimal, unit string, err error) {
	sum = decimal.Zero
	for _, cost := range o.Costs {
		sum = sum.Add(cost.Cost)
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

func (o *OUCost) printCostPerAccount(awsClient awsprovider.Client) {
	err := o.getCost(awsClient)
	if err != nil {
		fmt.Println("Error while calling getCost(): ", err.Error())
		return
	}

	for _, accountCost := range o.Costs {
		resp := listAccountCostResponse{
			OU:        *o.OU.Id,
			AccountId: accountCost.AccountID,
			Cost:      accountCost.Cost,
			Unit:      accountCost.Unit,
		}
		if o.options.csv {
			fmt.Printf("%s,%s,%s,%s\n", *o.OU.Id, accountCost.AccountID, accountCost.Cost.StringFixed(2), accountCost.Unit)
			continue
		}
		err := outputflag.PrintResponse(o.options.output, resp)
		if err != nil {
			fmt.Println("Error while printing response: ", err.Error())
			return
		}
	}
	sum, unit, err := o.getSum() // Sum up account costs
	if err != nil {
		log.Fatalln("Error summing up cost of OU:", err)
	}
	if o.options.csv {
		if o.options.sum {
			fmt.Printf("%s,%s,%s,%s\n", *o.OU.Id, "SUM", sum.StringFixed(2), unit)
		}
		return
	}
	printCostList(sum, unit, o.OU, o.options, true)
}

func printCostList(cost decimal.Decimal, unit string, OU *types.OrganizationalUnit, ops *listOptions, isChildNode bool) {

	resp := listCostResponse{
		OuId:    *OU.Id,
		OuName:  *OU.Name,
		CostUSD: cost,
	}

	if !isChildNode {
		return
	}

	if ops.csv {
		fmt.Printf("%v,%v,%s,%s\n", *OU.Id, *OU.Name, cost.StringFixed(2), unit)
		return
	}

	err := outputflag.PrintResponse(ops.output, resp)
	if err != nil {
		fmt.Println("Error while printing response: ", err.Error())
		return
	}
}
