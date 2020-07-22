package cost

import (
	"fmt"
	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
	"github.com/spf13/cobra"
	"log"
	"strconv"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
)

// getCmd represents the get command
func newCmdGet(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newGetOptions(streams)
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get total cost of a given OU. If no OU given, then gets total cost of v4 OU.",
		Run: func(cmd *cobra.Command, args []string) {

			awsClient, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			//Get information regarding Organizational Unit
			OU := getOU(awsClient, ops.ou)

			var cost float64
			var unit string

			if ops.recursive {
				if err := getOUCostRecursive(&cost, OU, awsClient, &ops.time); err != nil {
					log.Fatalln("Error getting cost of OU recursively:", err)
				}
				fmt.Printf("Cost of %s OU recursively is: %f%s\n", *OU.Name, cost, unit)
			} else {
				if err := getOUCost(&cost, OU, awsClient, &ops.time); err != nil {
					log.Fatalln("Error getting cost of OU:", err)
				}
				fmt.Printf("Cost of %s OU is: %f%s\n", *OU.Name, cost, unit)
			}
		},
	}
	getCmd.Flags().BoolVarP(&ops.recursive, "recursive", "r", false, "recurse through OUs")
	getCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time")
	getCmd.Flags().StringVar(&ops.ou, "ou", "", "get OU ID")

	if err := getCmd.MarkFlagRequired("ou"); err != nil {
		log.Fatalln("OU flag:", err)
	}

	return getCmd
}

//Store flag options for get command
type getOptions struct {
	ou        string
	recursive bool
	time      string

	genericclioptions.IOStreams
}

func newGetOptions(streams genericclioptions.IOStreams) *getOptions {
	return &getOptions{
		IOStreams: streams,
	}
}

//Get account IDs of immediate accounts under given OU
func getAccounts(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) ([]*string, error) {
	var accountSlice []*string
	var nextToken *string

	//Populate accountSlice with accounts by looping until accounts.NextToken is null
	for {
		accounts, err := awsClient.ListAccountsForParent(&organizations.ListAccountsForParentInput{
			ParentId:  OU.Id,
			NextToken: nextToken,
		})
		if err != nil {
			return nil, err
		}

		for i := 0; i < len(accounts.Accounts); i++ {
			accountSlice = append(accountSlice, accounts.Accounts[i].Id)
		}

		if accounts.NextToken == nil {
			break
		}
		nextToken = accounts.NextToken //If NextToken != nil, keep looping
	}

	return accountSlice, nil
}

//Get the account IDs of all (not only immediate) accounts under OU
func getAccountsRecursive(OU *organizations.OrganizationalUnit, awsClient awsprovider.Client) ([]*string, error) {
	var accountsIDs []*string

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
	accountsIDsOU, err := getAccounts(OU, awsClient)
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
func getAccountCost(accountID *string, awsClient awsprovider.Client, timePtr *string, cost *float64) error {
	//Starting from the 1st of the current month last year i.e. if today is 2020-06-29, then start date is 2019-06-01
	start := strconv.Itoa(time.Now().Year()-1) + time.Now().Format("-01-") + "01"
	end := time.Now().Format("2006-01-02")
	granularity := "MONTHLY"
	metrics := []string{
		"NetUnblendedCost",
	}

	switch *timePtr {
	case "MTD":
		start = time.Now().Format("2006-01") + "-01"
		end = time.Now().Format("2006-01-02")
	case "YTD":
		start = time.Now().Format("2006") + "-01-01"
		end = time.Now().Format("2006-01-02")
	case "TestError":
		start = "2020-05-23"
		end = "2019-06-12"
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
		monthCost, err := strconv.ParseFloat(*costs.ResultsByTime[month].Total["NetUnblendedCost"].Amount, 64)
		if err != nil {
			return err
		}
		*cost += monthCost
	}

	return nil
}

//Get cost of given OU by aggregating costs of only immediate accounts under given OU
func getOUCost(cost *float64, OU *organizations.OrganizationalUnit, awsClient awsprovider.Client, timePtr *string) error {
	//Populate accounts
	accounts, err := getAccounts(OU, awsClient)
	if err != nil {
		return err
	}

	//Increment costs of accounts
	for _, account := range accounts {
		if err := getAccountCost(account, awsClient, timePtr, cost); err != nil {
			return err
		}
	}

	return nil
}

//Get cost of given OU by aggregating costs of all (including immediate) accounts under OU
func getOUCostRecursive(cost *float64, OU *organizations.OrganizationalUnit, awsClient awsprovider.Client, timePtr *string) error {
	//Populate OUs
	OUs, err := getOUs(OU, awsClient)
	if err != nil {
		return err
	}

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		if err := getOUCostRecursive(cost, childOU, awsClient, timePtr); err != nil {
			return err
		}
	}

	//Return cost of child OUs + cost of immediate accounts under current OU
	if err := getOUCost(cost, OU, awsClient, timePtr); err != nil {
		return err
	}

	return nil
}
