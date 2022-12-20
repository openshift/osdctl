package org

import (
	"fmt"
	"log"
	"os"

	acc_util "github.com/openshift-online/ocm-cli/pkg/account"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var (
	usersCmd = &cobra.Command{
		Use:           "users",
		Short:         "get oraganization users",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(checkUsersOrgId(cmd, args))
			cmdutil.CheckErr(GetUsers(args[0]))
		},
	}
)

type userModel struct {
	userName string
	userID   string
}

var args struct {
	debug bool
	org   string
	roles []string
}

func checkUsersOrgId(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		err := cmd.Help()
		if err != nil {
			return fmt.Errorf("error calling cmd.Help(): %w", err)

		}
		return fmt.Errorf("oraganization id was not provided. please provide a oraganization id")
	}

	return nil
}

func checkRoles(roles, roleArgs []string) bool {
	for _, role := range roles {
		for _, roleArg := range roleArgs {
			if role == roleArg {
				return true
			}
		}
	}
	return false
}

// printArray turns an array into a string
func printArray(arrStr []string) string {
	var finalString string
	for item := range arrStr {
		finalString = fmt.Sprint(arrStr[item], " ", finalString)
	}
	return finalString
}

func GetUsers(orgID string) error {
	pageSize := 100
	pageIndex := 1
	searchQuery := ""

	searchQuery = fmt.Sprintf("organization_id='%s'", orgID)
	ocmClient := utils.CreateConnection()

	// Print top.
	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"USER", "USER ID", "ROLES"})

	for {
		// Get all users within organization
		usersResponse, err := ocmClient.AccountsMgmt().V1().Accounts().List().
			Size(pageSize).
			Page(pageIndex).
			Parameter("search", searchQuery).
			Send()
		if err != nil {
			log.Fatalf("Can't retrieve accounts: %v", err)
		}

		accountList := []*amv1.Account{}
		accountMap := make(map[*amv1.Account]*userModel)

		usersResponse.Items().Each(func(account *amv1.Account) bool {
			accountList = append(accountList, account)
			accountMap[account] = &userModel{
				userName: account.Username(),
				userID:   account.ID(),
			}
			return true
		})

		accountRoleMap, err := acc_util.GetRolesFromUsers(accountList, ocmClient)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get roles for user: %s\n", err)
			os.Exit(1)
		}

		for k, v := range accountRoleMap {
			if len(args.roles) > 0 {
				if checkRoles(v, args.roles) {
					table.AddRow([]string{
						accountMap[k].userName,
						accountMap[k].userID,
						printArray(v),
					})
				}
			} else {
				table.AddRow([]string{
					accountMap[k].userName,
					accountMap[k].userID,
					printArray(v),
				})
			}
		}

		if usersResponse.Size() < pageSize {
			break
		}
		pageIndex++
	}

	table.AddRow([]string{})
	table.Flush()

	return nil
}
