package cost

import (
	"context"
	"fmt"
	"log"
	"sort"

	awsv1alpha1 "github.com/openshift/aws-account-operator/pkg/apis/aws/v1alpha1"
	accountget "github.com/openshift/osdctl/cmd/account/get"
	"github.com/openshift/osdctl/cmd/common"
	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/shopspring/decimal"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/service/organizations"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// listCmd represents the list command
func newCmdList(streams genericclioptions.IOStreams, kubeCli k8sclient.Client, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newListOptions(streams, kubeCli, globalOpts)
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
	listCmd.Flags().BoolVar(&ops.claims, "claims", false, "Find matching AccountClaims in currently logged in cluster (SLOW!)")

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

//Store flag options for get command
type listOptions struct {
	ou     []string
	time   string
	start  string
	end    string
	level  string
	csv    bool
	sum    bool
	claims bool
	output string

	genericclioptions.IOStreams
	kubeCli       client.Client
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

func newListOptions(streams genericclioptions.IOStreams, kubeCli client.Client, globalOpts *globalflags.GlobalOptions) *listOptions {
	return &listOptions{
		IOStreams:     streams,
		kubeCli:       kubeCli,
		GlobalOptions: globalOpts,
	}
}

func (ops *listOptions) runList() error {
	awsClient, err := opsCost.initAWSClients()
	cmdutil.CheckErr(err)

	printHeader(ops)

	for _, ou := range ops.ou {
		OU := getOU(awsClient, ou)

		var cost decimal.Decimal
		var unit string

		if ops.level == "ou" {
			if err := listCostsUnderOU(OU, awsClient, ops); err != nil {
				log.Fatalln("Error listing costs under OU:", err)
			}
			printCostList(cost, unit, OU, ops, true) // TODO: Update bool here
			return nil
		}

		if ops.level == "account" {
			ouCost := OUCost{
				OU:      OU,
				options: ops,
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
			header := "OU,AccountID,Cost,Unit"
			if ops.claims {
				header = "Namespace,AccountClaimName," + header
			}
			fmt.Println(header)
		}
	case "ou":
		if ops.csv {
			fmt.Printf("OU,Name,Cost,Unit\n")
			break
		}
	}
}

//List the cost of each OU under given OU
func listCostsUnderOU(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client, ops *listOptions) error {
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
	AccountID    string
	Cost         decimal.Decimal
	Unit         string
	accountClaim awsv1alpha1.AccountClaim
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

func (o OUCost) getSum() (sum decimal.Decimal, unit string, err error) {
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

func (o *OUCost) getAccountClaims(kubeCli client.Client) {
	for i, cost := range o.Costs {
		accountclaim, err := accountget.GetAccountClaimFromAccountID(context.TODO(), o.options.kubeCli, cost.AccountID, common.AWSAccountNamespace)
		if err == nil {
			o.Costs[i].accountClaim = accountclaim
		}
	}
}

func (o OUCost) printCostPerAccount(awsClient awsprovider.Client) {
	o.getCost(awsClient)
	if o.options.claims {
		o.getAccountClaims(o.options.kubeCli)
	}

	for _, accountCost := range o.Costs {
		resp := listAccountCostResponse{
			OU:        *o.OU.Id,
			AccountId: accountCost.AccountID,
			Cost:      accountCost.Cost,
			Unit:      accountCost.Unit,
		}
		if o.options.csv {
			if o.options.claims {
				fmt.Printf("%s,%s,%s,%s,%s,%s\n", accountCost.accountClaim.Namespace, accountCost.accountClaim.Name, *o.OU.Id, accountCost.AccountID, accountCost.Cost.StringFixed(2), accountCost.Unit)
				continue
			}
			fmt.Printf("%s,%s,%s,%s\n", *o.OU.Id, accountCost.AccountID, accountCost.Cost.StringFixed(2), accountCost.Unit)
			continue
		}
		outputflag.PrintResponse(o.options.output, resp)
	}
	sum, unit, err := o.getSum() // Sum up account costs
	if err != nil {
		log.Fatalln("Error summing up cost of OU:", err)
	}
	if o.options.csv {
		if o.options.sum {
			output := fmt.Sprintf("%s,%s,%s,%s", *o.OU.Id, "SUM", sum.StringFixed(2), unit)
			if o.options.claims {
				output = ",," + output
			}
			fmt.Println(output)
		}
		return
	}
	printCostList(sum, unit, o.OU, o.options, true)
}

func printCostList(cost decimal.Decimal, unit string, OU *organizations.OrganizationalUnit, ops *listOptions, isChildNode bool) {

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

	outputflag.PrintResponse(ops.output, resp)
}
