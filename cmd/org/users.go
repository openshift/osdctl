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
		Short:         "get organization users",
		Args:          cobra.ArbitraryArgs,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(checkOrgId(cmd, args))
			cmdutil.CheckErr(getUsers(args[0]))
		},
	}
)

type UserItems struct {
	Users []*userModel `json:"items"`
}

type userModel struct {
	UserName string   `json:"user-name"`
	UserID   string   `json:"user-id"`
	Roles    []string `json:"roles"`
}

var args struct {
	debug bool
	org   string
	roles []string
}

func init() {
	flags := usersCmd.Flags()

	AddOutputFlag(flags)
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

func getUsers(orgID string) error {
	pageSize := 100
	pageIndex := 1
	searchQuery := ""

	searchQuery = fmt.Sprintf("organization_id='%s'", orgID)
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	var userList []*userModel
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
				UserName: account.Username(),
				UserID:   account.ID(),
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
					accountMap[k].Roles = v
					userList = append(userList, accountMap[k])
				}
			} else {
				accountMap[k].Roles = v
				userList = append(userList, accountMap[k])
			}
		}

		if usersResponse.Size() < pageSize {
			break
		}
		pageIndex++
	}

	printUsers(userList)
	return nil
}

func printUsers(userList []*userModel) {
	if IsJsonOutput() {
		users := UserItems{
			Users: userList,
		}
		PrintJson(users)
	} else {
		table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
		table.AddRow([]string{"USER", "USER ID", "ROLES"})

		for _, user := range userList {
			table.AddRow([]string{
				user.UserName,
				user.UserID,
				printArray(user.Roles),
			})
		}

		table.AddRow([]string{})
		table.Flush()
	}

}
