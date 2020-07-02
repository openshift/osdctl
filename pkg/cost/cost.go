// snippet-sourceauthor:[tokiwong]
// snippet-sourcedescription:[Retrieves cost and usage metrics for your account]
// snippet-keyword:[Amazon Cost Explorer]
// snippet-keyword:[Amazon CE]
// snippet-keyword:[GetCostAndUsage function]
// snippet-keyword:[Go]
// snippet-sourcesyntax:[go]
// snippet-service:[ce]
// snippet-keyword:[Code Sample]
// snippet-sourcetype:[full-example]
// snippet-sourcedate:[2019-07-09]
/*
   Copyright 2010-2019 Amazon.com, Inc. or its affiliates. All Rights Reserved.
   This file is licensed under the Apache License, Version 2.0 (the "License").
   You may not use this file except in compliance with the License. A copy of
   the License is located at
    http://aws.amazon.com/apache2.0/
   This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
   CONDITIONS OF ANY KIND, either express or implied. See the License for the
   specific language governing permissions and limitations under the License.
*/

package cost

import (
	"fmt"
	"github.com/aws/aws-sdk-go/service/organizations"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/costexplorer"
)

func main() {

	//Must be in YYYY-MM-DD Format
	start := "2019-09-01"
	end := "2019-11-01"
	granularity := "MONTHLY"
	metrics := []string{
		"BlendedCost",
		"UnblendedCost",
		"UsageQuantity",
	}
	// Initialize a session in us-east-1 that the SDK will use to load credentials
	//sess, err := session.NewSession(&aws.Config{
	//	Region: aws.String("us-east-1")},
	//)

	//Initialize a session with the osd-staging-1 profile or any user that has access to the desired info
	sess, err := session.NewSessionWithOptions(session.Options{
		Profile: "osd-staging-1",
	})

	// Create Cost Explorer Service Client
	svc := costexplorer.New(sess)

	result, err := svc.GetCostAndUsage(&costexplorer.GetCostAndUsageInput{
		TimePeriod: &costexplorer.DateInterval{
			Start: aws.String(start),
			End:   aws.String(end),
		},
		Granularity: aws.String(granularity),
		GroupBy: []*costexplorer.GroupDefinition{
			&costexplorer.GroupDefinition{
				Type: aws.String("DIMENSION"),
				Key:  aws.String("SERVICE"),
			},
		},
		Metrics: aws.StringSlice(metrics),
	})
	if err != nil {
		exitErrorf("Unable to generate report, %v", err)
	}

	fmt.Println("Cost Report:", result.ResultsByTime)

	fmt.Println()
	fmt.Println()
	fmt.Println()
	fmt.Println()
	fmt.Println()


	//Accessing organizations
	org := organizations.New(sess)

	//Get OU
	v4 := organizations.ListAccountsForParentInput{
		ParentId:   aws.String("ou-0wd6-oq5d7v8g"),
	}

	//Do DFS Post-Order traversal
	DFS(&v4, org)


	//acc, err := svcOrg.ListAccountsForParent(&OU)
	//if err != nil {
	//	exitErrorf("Unable to retrieve accounts under OU", err)
	//}
	//fmt.Println(acc, svcOrg)

}

func DFS(OU *organizations.ListAccountsForParentInput, org *organizations.Organizations) {
	//Check for errors
	result, err := org.ListAccountsForParent(OU)
	if err != nil {	//Look at this for error handling: https://docs.aws.amazon.com/sdk-for-go/api/service/organizations/#example_Organizations_ListOrganizationalUnitsForParent_shared00
		exitErrorf("Unable to retrieve accounts under OU", err)
	}

	OUs := &organizations.ListOrganizationalUnitsForParentInput{
		ParentId: OU.ParentId,
	}

	resultOUs, err := org.ListOrganizationalUnitsForParent(OUs)
	if err != nil {
		exitErrorf("Unable to retrieve child OUs under OU", err)
	}

	//Loop through all child OUs
	//for OUs.
	fmt.Println("These are the accounts:\n",result)
	fmt.Println("These are the OUs:\n",resultOUs)
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}
