package cost

import (
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/costexplorer"
	"github.com/aws/aws-sdk-go/service/organizations"
)

func main() {
	//Measure time
	startTime := time.Now()

	//Initialize a session with the osd-staging-1 profile or any user that has access to the desired info
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: "osd-staging-1",
	})
	if err != nil {
		log.Fatalln("Unable to generate session:", err)

	}

	//Create Cost Explorer client
	ce := costexplorer.New(sess)
	//Create Organizations client
	org := organizations.New(sess)

	//Get organizational unit
	OU := organizations.OrganizationalUnit{
		//Id:   aws.String("ou-0wd6-aff5ji37"), //v4
		Id: aws.String("ou-0wd6-3321fxfw"), //Test small OU
		//Id:   aws.String("ou-0wd6-k7wulboi"), //slightly larger small OU
		//Id:   aws.String("r-0wd6"), //Test root
		//Id:   aws.String("ou-0wd6-oq5d7v8g"), //Test for cost category
	}

	//Store cost of OU
	var cost float64 = 0

	//Set flag pointers
	rPtr := flag.Bool("r", false, "recurse")
	recursivePtr := flag.Bool("recursive", false, "recurse")
	timePtr := flag.String("time", "all", "set time")
	ccPtr := flag.String("ccc", "", "create cost category")
	//Parse pointers
	flag.Parse()

	if *ccPtr != "" {
		//Set OU to argument of flag -ccc.
		OU.Id = ccPtr
		//Then, create cost category for given OU
		createCostCategory(ccPtr, &OU, org, ce)
	} else if *rPtr || *recursivePtr { //If -r flag is present, do a DFS postorder traversal and get cost of all accounts under OU
		getOUCostRecursive(&OU, org, ce, timePtr, &cost)
	} else { //Else, get cost of only immediate accounts under OU
		getOUCost(&OU, org, ce, timePtr, &cost)
	}

	fmt.Println("Recursive cost of OU:", cost)

	//End time
	endTime := time.Now()
	fmt.Println("Time of program execution:", endTime.Sub(startTime))
}

//Create Cost Category for OU given as argument for -ccc flag
func createCostCategory(OUid *string, OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer) {
	accounts := getOUAccountsRecursive(OU, org)

	_, err := ce.CreateCostCategoryDefinition(&costexplorer.CreateCostCategoryDefinitionInput{
		Name:        OUid,
		RuleVersion: aws.String("CostCategoryExpression.v1"),
		Rules: []*costexplorer.CostCategoryRule{
			{
				Rule: &costexplorer.Expression{
					Dimensions: &costexplorer.DimensionValues{
						Key:    aws.String("LINKED_ACCOUNT"),
						Values: accounts,
					},
				},
				Value: OUid,
			},
		},
	})
	if err != nil {
		log.Fatalln("Error creating cost category:", err)
	}
}

//Get cost of immediate accounts under given OU
func getAccountCost(accountID *string, ce *costexplorer.CostExplorer, timePtr *string, cost *float64) {

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

//Get account IDs from immediate accounts under given OU
func getAccountsIDs(OU *organizations.OrganizationalUnit, org *organizations.Organizations) []*string {
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

//Get cost of immediate accounts under given OU
func getOUCost(OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer, timePtr *string, cost *float64) {
	//Populate accounts
	accounts := getAccountsIDs(OU, org)

	//Increment costs of accounts
	for _, account := range accounts {
		getAccountCost(account, ce, timePtr, cost)
	}
}

//Get immediate OUs (child nodes) directly under given OU
func getOUs(OU *organizations.OrganizationalUnit, org *organizations.Organizations) []*organizations.OrganizationalUnit {
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

//Get cost of all (not only immediate) accounts under OU
func getOUCostRecursive(OU *organizations.OrganizationalUnit, org *organizations.Organizations, ce *costexplorer.CostExplorer, timePtr *string, cost *float64) {
	//Populate OUs
	OUs := getOUs(OU, org)

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		getOUCostRecursive(childOU, org, ce, timePtr, cost)
	}

	//Return cost of child OUs + cost of immediate accounts under current OU
	getOUCost(OU, org, ce, timePtr, cost)
}

//Get the account ID of all (not only immediate) accounts under OU
func getOUAccountsRecursive(OU *organizations.OrganizationalUnit, org *organizations.Organizations) []*string {
	var accountsIDs []*string

	//Populate OUs
	OUs := getOUs(OU, org)

	//Loop through all child OUs, get their costs, and store it to cost of current OU
	for _, childOU := range OUs {
		accountsIDs = append(accountsIDs, getOUAccountsRecursive(childOU, org)...)
	}

	//*accountsIDs = append(*accountsIDs, getAccountsIDs(OU, org)...)
	return append(accountsIDs, getAccountsIDs(OU, org)...)
}
