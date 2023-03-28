package aao

import (
	"context"
	"fmt"
	"sort"

	awsv1alpha1 "github.com/openshift/aws-account-operator/api/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// newCmdPool gets the current status of the AWS Account Operator AccountPool
func newCmdPool(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, client client.Client) *cobra.Command {
	ops := newPoolOptions(streams, flags, client)
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

func newPoolOptions(streams genericclioptions.IOStreams, _ *genericclioptions.ConfigFlags, client client.Client) *poolOptions {
	return &poolOptions{
		IOStreams: streams,
		kubeCli:   client,
	}
}

func (o *poolOptions) complete(cmd *cobra.Command) error {
	return nil
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
	defaultMap := make(map[string]int)
	fmMap := make(map[string]int)
	availabilityCount := 0
	for _, account := range accounts.Items {

		if !account.Status.Claimed && account.Status.State == "Ready" && account.Spec.LegalEntity.ID == "" {
			availabilityCount += 1
		}

		if account.Spec.LegalEntity.ID == "" {
			continue
		}

		key := fmt.Sprintf("%s %s", account.Spec.LegalEntity.ID, account.Spec.LegalEntity.Name)
		if account.Spec.AccountPool == "fm-accountpool" { // non-default accountpool
			fmMap[key] += 1
		} else {
			defaultMap[key] += 1
		}
	}
	fmt.Printf("Available Accounts: %d\n", availabilityCount)
	fmt.Println("============================================================")
	fmt.Println("Default Account Pool")
	fmt.Println("============================================================")
	printSortedCount(getSortedCount(defaultMap, 10))
	fmt.Println()

	fmt.Println("============================================================")
	fmt.Println("fm-accountpool")
	fmt.Println("============================================================")
	printSortedCount(getSortedCount(fmMap, 10))
	fmt.Println()

	return nil
}

type legalEntityCount struct {
	legalEntity string
	count       int
}

func mapToCountArray(input map[string]int) []legalEntityCount {
	result := make([]legalEntityCount, 0)
	for k, v := range input {
		result = append(result, legalEntityCount{legalEntity: k, count: v})
	}

	return result
}

func getSortedCount(inputMap map[string]int, maxLen int) []legalEntityCount {
	inputCounts := mapToCountArray(inputMap)
	sort.Slice(inputCounts, func(i, j int) bool {
		return inputCounts[i].count > inputCounts[j].count
	})

	limit := len(inputCounts)
	if maxLen > limit {
		return inputCounts[:limit]
	}

	return inputCounts[:maxLen]
}

func printSortedCount(lec []legalEntityCount) {
	for i := range lec {
		fmt.Printf("%d:\t%s \n", lec[i].count, lec[i].legalEntity)
	}
}
