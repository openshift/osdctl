package cost

import (
	"fmt"
	"log"
	"strconv"
	"time"

	outputflag "github.com/openshift/osdctl/cmd/getoutput"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	awsprovider "github.com/openshift/osdctl/pkg/provider/aws"
	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
)

// getCmd represents the get command
func newCmdGet(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newGetOptions(streams, globalOpts)
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get total cost of a given OU",
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.checkArgs(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	getCmd.Flags().StringVar(&ops.ou, "ou", "", "set OU ID")
	getCmd.Flags().BoolVarP(&ops.recursive, "recursive", "r", false, "recurse through OUs")
	getCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time. One of 'LM', 'MTD', 'YTD', '3M', '6M', '1Y'")
	getCmd.Flags().StringVar(&ops.start, "start", "", "set start date range")
	getCmd.Flags().StringVar(&ops.end, "end", "", "set end date range")
	getCmd.Flags().BoolVar(&ops.csv, "csv", false, "output result as csv")

	return getCmd
}

func (o *getOptions) checkArgs(cmd *cobra.Command, _ []string) error {

	// If no date range or time is define error out
	if o.start == "" && o.end == "" && o.time == "" {
		return cmdutil.UsageErrorf(cmd, "Please provide a date range or a predefined time")
	}
	// If both date range and time are defined error out
	if o.start != "" && o.end != "" && o.time != "" {
		return cmdutil.UsageErrorf(cmd, "Please provide either a date range or a predefined time")
	}
	// If either start or end is missing error out
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
type getOptions struct {
	ou        string
	recursive bool
	time      string
	start     string
	end       string
	csv       bool
	output    string

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

type getCostResponse struct {
	OuId    string          `json:"ouid" yaml:"ouid"`
	OuName  string          `json:"ouname" yaml:"ouname"`
	CostUSD decimal.Decimal `json:"costUSD" yaml:"costUSD"`
}

func (f getCostResponse) String() string {

	return fmt.Sprintf("  OuId: %s\n  OuName: %s\n  Cost: %s\n", f.OuId, f.OuName, f.CostUSD)

}

func newGetOptions(streams genericclioptions.IOStreams, globalOpts *globalflags.GlobalOptions) *getOptions {
	return &getOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *getOptions) run() error {

	awsClient, err := opsCost.initAWSClients()
	if err != nil {
		return err
	}

	//Get information regarding Organizational Unit
	OU := getOU(awsClient, o.ou)

	var cost decimal.Decimal
	var unit string

	if o.recursive { //Get cost of given OU by aggregating costs of all (including immediate) accounts under OU
		if err := o.getOUCostRecursive(&cost, &unit, OU, awsClient); err != nil {
			log.Fatalln("Error getting cost of OU recursively:", err)
		}
	} else { //Get cost of given OU by aggregating costs of only immediate accounts under given OU
		if err := o.getOUCost(&cost, &unit, OU, awsClient); err != nil {
			log.Fatalln("Error getting cost of OU:", err)
		}
	}

	o.printCostGet(cost, unit, o, OU)
	return nil
}

//Get account IDs of immediate accounts under given OU
func getAccounts(ouId string, awsClient awsprovider.Client) ([]AccountOU, error) {
	var accountSlice []AccountOU
	var nextToken *string

	//Populate accountSlice with accounts by looping until accounts.NextToken is null
	for {
		accounts, err := awsClient.ListAccountsForParent(&organizations.ListAccountsForParentInput{
			ParentId:  &ouId,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(accounts.Accounts); i++ {
			accountSlice = append(accountSlice, AccountOU{*accounts.Accounts[i].Id, ouId})
		}

		if accounts.NextToken == nil {
			break
		}
		nextToken = accounts.NextToken //If NextToken != nil, keep looping
		fmt.Printf("%c[2K\rRead %d accounts.", 27, len(accountSlice))
	}
	fmt.Printf("\ndone\n")

	return accountSlice, nil
}

type AccountOU struct {
	accountId string
	ouId      string
}

//Get the account IDs of all (not only immediate) accounts under OU
func getAccountsRecursive(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) ([]AccountOU, error) {
	var accountsIDs []AccountOU

	//Populate OUs
	OUs, err := getOUs(OU, awsClient)
	if err != nil {
		return nil, err
	}

	//Loop through all child OUs to get account IDs from the accounts that comprise the OU
	for _, childOU := range OUs {
		accountsIDsOU, _ := getAccountsRecursive(childOU, awsClient)
		accountsIDs = append(accountsIDs, accountsIDsOU...)
	}
	//Get account
	accountsIDsOU, err := getAccounts(*OU.Id, awsClient)
	if err != nil {
		return nil, err
	}

	return append(accountsIDs, accountsIDsOU...), nil
}

//Get immediate OUs (child nodes) directly under given OU
func getOUs(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) ([]*organizations.OrganizationalUnit, error) {
	var OUSlice []*organizations.OrganizationalUnit
	var nextToken *string

	//Populate OUSlice with OUs by looping until OUs.NextToken is null
	for {
		OUs, err := awsClient.ListOrganizationalUnitsForParent(&organizations.ListOrganizationalUnitsForParentInput{
			ParentId:  OU.Id,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		//Add OUs to slice
		for childOU := 0; childOU < len(OUs.OrganizationalUnits); childOU++ {
			OUSlice = append(OUSlice, OUs.OrganizationalUnits[childOU])
		}

		if OUs.NextToken == nil {
			break
		}
		nextToken = OUs.NextToken //If NextToken != nil, keep looping
	}

	return OUSlice, nil
}

//Get the account IDs of all (not only immediate) accounts under OU
func getOUsRecursive(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) ([]*organizations.OrganizationalUnit, error) {
	var OUs []*organizations.OrganizationalUnit

	//Populate OUs by getting immediate OUs (direct nodes)
	currentOUs, err := getOUs(OU, awsClient)
	if err != nil {
		return nil, err
	}

	//Loop through all child OUs. Append the child OU, then append the OUs of the child OU
	for _, currentOU := range currentOUs {
		OUs = append(OUs, currentOU)

		OUsRecursive, _ := getOUsRecursive(currentOU, awsClient)
		OUs = append(OUs, OUsRecursive...)
	}

	return OUs, nil
}

//Get cost of given account
func (o *getOptions) getAccountCost(accountID *string, unit *string, awsClient awsprovider.Client, cost *decimal.Decimal) error {

	var start, end, granularity string
	if o.time != "" {
		start, end = getTimePeriod(&o.time)
		granularity = "MONTHLY"
	}

	if o.start != "" && o.end != "" {
		start = o.start
		end = o.end
		granularity = "DAILY"
	}

	metrics := []string{
		"NetUnblendedCost",
	}

	//Get cost information for chosen account
	costs, err := awsClient.GetCostAndUsage(&costexplorer.GetCostAndUsageInput{
		Filter: &costexplorer.Expression{
			Dimensions: &costexplorer.DimensionValues{
				Key: aws.String("LINKED_ACCOUNT"),
				Values: []*string{
					accountID,
				},
			},
		},
		TimePeriod: &costexplorer.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: aws.String(granularity),
		Metrics:     aws.StringSlice(metrics),
	})
	if err != nil {
		return err
	}

	//Loop through month-by-month cost and increment to get total cost
	for month := 0; month < len(costs.ResultsByTime); month++ {
		monthCost, err := decimal.NewFromString(*costs.ResultsByTime[month].Total["NetUnblendedCost"].Amount)
		if err != nil {
			return err
		}
		*cost = cost.Add(monthCost)
	}

	//Save unit
	*unit = *costs.ResultsByTime[0].Total["NetUnblendedCost"].Unit

	return nil
}

//Get cost of given OU by aggregating costs of only immediate accounts under given OU
func (o *getOptions) getOUCost(cost *decimal.Decimal, unit *string, OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) error {
	//Populate accounts
	accounts, err := getAccounts(*OU.Id, awsClient)
	if err != nil {
		return err
	}

	//Increment costs of accounts
	for _, account := range accounts {
		if err := o.getAccountCost(&account.accountId, unit, awsClient, cost); err != nil {
			return err
		}
	}

	return nil
}

//Get cost of given OU by aggregating costs of all (including immediate) accounts under OU
func (o *getOptions) getOUCostRecursive(cost *decimal.Decimal, unit *string, OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) error {
	//Populate OUs
	OUs, err := getOUs(OU, awsClient)
	if err != nil {
		return err
	}

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		if err := o.getOUCostRecursive(cost, unit, childOU, awsClient); err != nil {
			return err
		}
	}

	//Return cost of child OUs + cost of immediate accounts under current OU
	if err := o.getOUCost(cost, unit, OU, awsClient); err != nil {
		return err
	}

	return nil
}

//Get time period based on time flag
func getTimePeriod(timePtr *string) (string, string) {

	t := time.Now()

	//Starting from the 1st of the current month last year i.e. if today is 2020-06-29, then start date is 2019-06-01
	start := fmt.Sprintf("%d-%02d-%02d", t.Year()-1, t.Month(), 01)
	end := fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), t.Day())

	switch *timePtr {
	case "LM": //Last Month
		start = fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month()-1, 01)
		end = fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), 01)
	case "MTD":
		start = fmt.Sprintf("%d-%02d-%02d", t.Year(), t.Month(), 01)
	case "YTD":
		start = fmt.Sprintf("%d-%02d-%02d", t.Year(), 01, 01)
	case "3M":
		if month := t.Month(); month > 3 {
			start = t.AddDate(0, -3, 0).Format("2006-01-02")
		} else {
			start = t.AddDate(-1, 9, 0).Format("2006-01-02")
		}
	case "6M":
		if month, _ := strconv.Atoi(time.Now().Format("01")); month > 6 {
			start = t.AddDate(0, -6, 0).Format("2006-01-02")
		} else {
			start = t.AddDate(-1, 6, 0).Format("2006-01-02")
		}
	case "1Y":
		start = t.AddDate(-1, 0, 0).Format("2006-01-02")
	}

	return start, end
}

func (o *getOptions) printCostGet(cost decimal.Decimal, unit string, ops *getOptions, OU *organizations.OrganizationalUnit) error {

	resp := getCostResponse{
		OuId:    *OU.Id,
		OuName:  *OU.Name,
		CostUSD: cost,
	}

	if ops.csv { //If csv option specified, print result in csv
		fmt.Printf("\n%s,%s,%s\n\n", *OU.Name, cost.StringFixed(2), unit)
		return nil
	}
	if ops.recursive {
		fmt.Println("Cost of all accounts under OU:")
	}

	outputflag.PrintResponse(o.output, resp)

	return nil
}
