package cost

import (
	"fmt"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"

	"github.com/spf13/cobra"

	awsprovider "github.com/openshift/osd-utils-cli/pkg/provider/aws"
)

// getCmd represents the get command
func newCmdGet(streams genericclioptions.IOStreams) *cobra.Command {
	ops := newGetOptions(streams)
	getCmd := &cobra.Command{
		Use:   "get",
		Short: "Get total cost of a given OU. If no OU given, then gets total cost of v4 OU.",
		Run: func(cmd *cobra.Command, args []string) {

			cmdutil.CheckErr(opsCost.complete(cmd, args))
			org, ce, err := opsCost.initAWSClients()
			cmdutil.CheckErr(err)

			//Get Organizational Unit
			OU := organizations.OrganizationalUnit{Id: aws.String(ops.ou)}
			//Store cost
			var cost float64 = 0

			if ops.recursive {
				getOUCostRecursive(&OU, org, ce, &ops.time, &cost)
				fmt.Printf("Cost of %s recursively is: %f\n", ops.ou, cost)
			} else {
				getOUCost(&OU, org, ce, &ops.time, &cost)
				fmt.Printf("Cost of %s is: %f\n", ops.ou, cost)
			}
		},
	}
	getCmd.Flags().StringVar(&ops.ou, "ou", "ou-0wd6-aff5ji37", "get OU ID (default is v4)") //Default OU is v4
	getCmd.Flags().BoolVarP(&ops.recursive, "recursive", "r", false, "recurse through OUs")
	getCmd.Flags().StringVarP(&ops.time, "time", "t", "", "set time")

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
func getAccounts(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient) []*string {
	//accountSlice stores accounts
	var accountSlice []*string

	//Get accounts
	accounts, err := org.ListAccountsForParent(&organizations.ListAccountsForParentInput{
		ParentId: OU.Id,
	})

	//Populate accountSlice with accounts by looping until accounts.NextToken is null
	for {
		if err != nil { //Look at this for error handling: https://docs.aws.amazon.com/sdk-for-go/api/service/organizations/#example_Organizations_ListOrganizationalUnitsForParent_shared00
			log.Fatalln("Unable to retrieve accounts under OU:", err)
		}

		for i := 0; i < len(accounts.Accounts); i++ {
			accountSlice = append(accountSlice, accounts.Accounts[i].Id)
		}

		if accounts.NextToken == nil {
			break
		}

		//Get accounts
		accounts, err = org.ListAccountsForParent(&organizations.ListAccountsForParentInput{
			ParentId:  OU.Id,
			NextToken: accounts.NextToken,
		})
	}

	return accountSlice
}

//Get the account IDs of all (not only immediate) accounts under OU
func getAccountsRecursive(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient) []*string {
	var accountsIDs []*string

	//Populate OUs
	OUs := getOUs(OU, org)

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		accountsIDs = append(accountsIDs, getAccountsRecursive(childOU, org)...)
	}

	//*accountsIDs = append(*accountsIDs, getAccounts(OU, org)...)
	return append(accountsIDs, getAccounts(OU, org)...)
}

//Get immediate OUs (child nodes) directly under given OU
func getOUs(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient) []*organizations.OrganizationalUnit {
	//OUSlice stores OUs
	var OUSlice []*organizations.OrganizationalUnit

	//Get child OUs under parent OU
	OUs, err := org.ListOrganizationalUnitsForParent(&organizations.ListOrganizationalUnitsForParentInput{
		ParentId: OU.Id,
	})

	//Populate OUSlice with OUs by looping until OUs.NextToken is null
	for {
		if err != nil {
			log.Fatalln("Unable to retrieve child OUs under OU:", err)
		}

		//Add OUs to slice
		for childOU := 0; childOU < len(OUs.OrganizationalUnits); childOU++ {
			OUSlice = append(OUSlice, OUs.OrganizationalUnits[childOU])
		}

		if OUs.NextToken == nil {
			break
		}

		OUs, err = org.ListOrganizationalUnitsForParent(&organizations.ListOrganizationalUnitsForParentInput{
			ParentId:  OU.Id,
			NextToken: OUs.NextToken,
		})
	}

	return OUSlice
}

//Get the account IDs of all (not only immediate) accounts under OU
func getOUsRecursive(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient) []*organizations.OrganizationalUnit {
	var OUs []*organizations.OrganizationalUnit

	//Populate OUs by getting immediate OUs (direct nodes)
	currentOUs := getOUs(OU, org)

	//Loop through all child OUs. Append the child OU, then append the OUs of the child OU
	for _, currentOU := range currentOUs {
		OUs = append(OUs, currentOU)
		OUs = append(OUs, getOUsRecursive(currentOU, org)...)
	}

	return OUs
}

//Get cost of given account
func getAccountCost(accountID *string, ce awsprovider.CostExplorerClient, timePtr *string, cost *float64) {

	start := strconv.Itoa(time.Now().Year()-1) + time.Now().Format("-01-") + "01" //Starting from the 1st of the current month last year i.e. if today is 2020-06-29, then start date is 2019-06-01
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
	costs, err := ce.GetCostAndUsage(&costexplorer.GetCostAndUsageInput{
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
		log.Fatalln("Error getting costs report:", err)
	}

	//Loop through month-by-month cost and increment to get total cost
	for month := 0; month < len(costs.ResultsByTime); month++ {
		monthCost, err := strconv.ParseFloat(*costs.ResultsByTime[month].Total["NetUnblendedCost"].Amount, 64)
		if err != nil {
			log.Fatalln("Unable to get cost:", err)
		}
		*cost += monthCost
	}
}

//Get cost of given OU by aggregating costs of immediate accounts under given OU
func getOUCost(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient, ce awsprovider.CostExplorerClient, timePtr *string, cost *float64) {
	//Populate accounts
	accounts := getAccounts(OU, org)

	//Increment costs of accounts
	for _, account := range accounts {
		getAccountCost(account, ce, timePtr, cost)
	}
}

//Get cost of all (not only immediate) accounts under OU
func getOUCostRecursive(OU *organizations.OrganizationalUnit, org awsprovider.OrganizationsClient, ce awsprovider.CostExplorerClient, timePtr *string, cost *float64) {
	//Populate OUs
	OUs := getOUs(OU, org)

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		getOUCostRecursive(childOU, org, ce, timePtr, cost)
	}

	//Return cost of child OUs + cost of immediate accounts under current OU
	getOUCost(OU, org, ce, timePtr, cost)
}
