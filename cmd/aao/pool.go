package aao

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func newCmdPool(client client.Client) *cobra.Command {
	ops := newPoolOptions(client)
	poolCmd := &cobra.Command{
		Use:               "pool",
		Short:             "Get the status of the AWS Account Operator AccountPool",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd))
			cmdutil.CheckErr(ops.run())
		},
	}

	return poolCmd
}

// poolOptions defines the struct for running the pool command
type poolOptions struct {
	genericclioptions.IOStreams
	kubeCli client.Client
}

func newPoolOptions(client client.Client) *poolOptions {
	return &poolOptions{
		kubeCli: client,
	}
}

func (o *poolOptions) complete(cmd *cobra.Command) error {
	return nil
}

type legalEntityStats struct {
	name         string
	id           string
	claimedCount int
	unusedCount  int
	totalCount   int
}

func (o *poolOptions) run() error {
	ctx := context.TODO()
	var accounts awsv1alpha1.AccountList
	if err := o.kubeCli.List(ctx, &accounts, &client.ListOptions{
		Namespace: "aws-account-operator",
	}); err != nil {
		return err
	}

	// mapping legalentityid to count
	defaultMap := make(map[string]legalEntityStats)
	fmMap := make(map[string]legalEntityStats)
	availabilityCount := 0

	for _, account := range accounts.Items {

		if !account.Status.Claimed && account.Status.State == "Ready" && account.Spec.LegalEntity.ID == "" && !account.Spec.BYOC {
			availabilityCount += 1
		}

		if account.Spec.LegalEntity.ID == "" || account.Spec.BYOC {
			continue
		}

		if account.Spec.AccountPool == "fm-accountpool" { // non-default accountpool
			handlePoolCounting(fmMap, account)
		} else {
			handlePoolCounting(defaultMap, account)
		}
	}
	fmt.Fprintf(o.IOStreams.Out, "Available Accounts: %d\n", availabilityCount)
	fmt.Fprintf(o.IOStreams.Out, "========================================================================================================================")
	fmt.Fprintf(o.IOStreams.Out, "Default Account Pool")
	fmt.Fprintf(o.IOStreams.Out, "========================================================================================================================")
	printSortedCount(getSortedCount(defaultMap, 10), o.IOStreams.Out)

	fmt.Fprintln(o.IOStreams.Out, "========================================================================================================================")
	fmt.Fprintln(o.IOStreams.Out, "fm-accountpool")
	fmt.Fprintln(o.IOStreams.Out, "========================================================================================================================")
	printSortedCount(getSortedCount(fmMap, 10), o.IOStreams.Out)

	return nil
}

func handlePoolCounting(myMap map[string]legalEntityStats, account awsv1alpha1.Account) {
	key := fmt.Sprintf("%s %s", account.Spec.LegalEntity.ID, account.Spec.LegalEntity.Name)
	if account.Status.Claimed {
		if entry, ok := myMap[key]; ok {
			entry.claimedCount += 1
			myMap[key] = entry
		} else {
			myMap[key] = legalEntityStats{
				name:         account.Spec.LegalEntity.Name,
				id:           account.Spec.LegalEntity.ID,
				claimedCount: 1,
			}
		}
	} else {
		if entry, ok := myMap[key]; ok {
			entry.unusedCount += 1
			myMap[key] = entry
		} else {
			myMap[key] = legalEntityStats{
				name:        account.Spec.LegalEntity.Name,
				id:          account.Spec.LegalEntity.ID,
				unusedCount: 1,
			}
		}
	}
}

func prepareLESMapToArray(input map[string]legalEntityStats) []legalEntityStats {
	result := make([]legalEntityStats, 0)
	for _, v := range input {
		v.totalCount = v.claimedCount + v.unusedCount
		result = append(result, v)
	}

	return result
}

func getSortedCount(inputMap map[string]legalEntityStats, maxLen int) []legalEntityStats {
	inputCounts := prepareLESMapToArray(inputMap)
	sort.Slice(inputCounts, func(i, j int) bool {
		return inputCounts[i].totalCount > inputCounts[j].totalCount
	})

	limit := len(inputCounts)
	if maxLen > limit {
		return inputCounts[:limit]
	}

	return inputCounts[:maxLen]
}

func printSortedCount(lec []legalEntityStats, out io.Writer) {

	totalClaimed := 0
	totalUnused := 0
	totalTotal := 0

	table := printer.NewTablePrinter(os.Stdout, 20, 1, 3, ' ')
	table.AddRow([]string{"Claimed", "Unused", "Total", "ID", "Name"})
	for i := range lec {
		table.AddRow([]string{
			strconv.Itoa(lec[i].claimedCount),
			strconv.Itoa(lec[i].unusedCount),
			strconv.Itoa(lec[i].totalCount),
			lec[i].id, lec[i].name,
		})
		totalClaimed += lec[i].claimedCount
		totalUnused += lec[i].unusedCount
		totalTotal += lec[i].totalCount
	}

	// Add empty row for readability
	table.AddRow([]string{})
	err := table.Flush()
	if err != nil {
		fmt.Fprintln(out, "error while flushing table: ", err.Error())
	}

	fmt.Fprintf(out, "Total Claimed: %d\n", totalClaimed)
	fmt.Fprintf(out, "Total Unused: %d\n", totalUnused)
	fmt.Fprintf(out, "Total Accounts: %d\n", totalTotal)
	fmt.Fprintln(out)
}
