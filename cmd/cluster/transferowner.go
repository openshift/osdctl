package cluster

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/openshift-online/ocm-cli/pkg/arguments"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/openshift/osdctl/internal/utils/globalflags"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// transferOwnerOptions defines the struct for running transferOwner command
type transferOwnerOptions struct {
	output            string
	clusterID         string
	newOwnerName      string
	newOrganizationId string
	oldOwnerName      string
	oldOrganizationId string
	dryrun            bool

	genericclioptions.IOStreams
	GlobalOptions *globalflags.GlobalOptions
}

func newCmdTransferOwner(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *cobra.Command {
	ops := newTransferOwnerOptions(streams, flags, globalOpts)
	transferOwnerCmd := &cobra.Command{
		Use:               "transferOwner",
		Short:             "Transfer cluster ownership to a new user (to be done by Region Lead)",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}
	// can we get cluster-id from some context maybe?
	transferOwnerCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "The Internal Cluster ID/External Cluster ID/ Cluster Name")
	transferOwnerCmd.Flags().StringVar(&ops.newOwnerName, "new-owner", ops.newOwnerName, "The new owners username to transfer the cluster to")
	transferOwnerCmd.Flags().StringVar(&ops.newOrganizationId, "new-organization-id", ops.newOrganizationId, "Organization of the new owner")
	transferOwnerCmd.Flags().StringVar(&ops.oldOwnerName, "old-owner", ops.oldOwnerName, "The old owners username to transfer the cluster from")
	transferOwnerCmd.Flags().StringVar(&ops.oldOrganizationId, "old-organization-id", ops.oldOrganizationId, "Organization of the old owner")
	transferOwnerCmd.Flags().BoolVarP(&ops.dryrun, "dry-run", "d", false, "Dry-run - show all changes but do not apply them")

	transferOwnerCmd.MarkFlagRequired("cluster-id")
	transferOwnerCmd.MarkFlagRequired("new-owner")
	transferOwnerCmd.MarkFlagRequired("old-owner")
	transferOwnerCmd.MarkFlagRequired("new-organization-id")
	transferOwnerCmd.MarkFlagRequired("old-organization-id")

	return transferOwnerCmd
}

func newTransferOwnerOptions(streams genericclioptions.IOStreams, flags *genericclioptions.ConfigFlags, globalOpts *globalflags.GlobalOptions) *transferOwnerOptions {
	return &transferOwnerOptions{
		IOStreams:     streams,
		GlobalOptions: globalOpts,
	}
}

func (o *transferOwnerOptions) complete(cmd *cobra.Command, _ []string) error {
	o.output = o.GlobalOptions.Output
	err := validateOrgID(o.newOrganizationId)
	if err != nil {
		return fmt.Errorf("error validating new organization-id: %w", err)
	}
	err = validateOrgID(o.oldOrganizationId)
	if err != nil {
		return fmt.Errorf("error validating old organization-id: %w", err)
	}
	return nil
}

// basic organization-id input validation
func validateOrgID(orgID string) error {
	ok, err := regexp.MatchString(`[a-zA-Z0-9]{27}`, orgID)
	if err != nil {
		return fmt.Errorf("error validating new organization-id")
	}
	if !ok {
		return fmt.Errorf("invalid new organization id provided: %v", orgID)
	}
	return nil
}

func (o *transferOwnerOptions) run() error {
	fmt.Print("Before making changes in OCM, the cluster must have the pull secret updated to be the new owner's. ")
	fmt.Print("See: https://github.com/openshift/ops-sop/blob/master/v4/howto/replace-pull-secret.md\n")
	err := utils.ConfirmSend()
	if err != nil {
		return err
	}

	// Create an OCM client to talk to the cluster API
	// the user has to be logged in (e.g. 'ocm login')
	ocm := utils.CreateConnection()
	defer func() {
		if err := ocm.Close(); err != nil {
			fmt.Printf("Cannot close the ocm (possible memory leak): %q", err)
		}
	}()

	// Find and setup all resources that are needed

	cluster, err := utils.GetCluster(ocm, o.clusterID)
	if err != nil {
		return err
	}

	externalClusterID, ok := cluster.GetExternalID()
	if !ok {
		return fmt.Errorf("cluster has no external id")
	}

	subscription, err := utils.GetSubscription(ocm, externalClusterID)
	if err != nil {
		return fmt.Errorf("could not get subscription: %w", err)
	}

	account, err := utils.GetAccount(ocm, o.newOwnerName)
	if err != nil {
		return fmt.Errorf("could not get new owners account: %w", err)
	}

	accountID, ok := account.GetID()
	if !ok {
		return fmt.Errorf("account has no id")
	}

	oldOwnerAccount, err := utils.GetAccount(ocm, o.oldOwnerName)
	if err != nil {
		return fmt.Errorf("could not get old owners account: %w", err)
	}

	subscriptionID, ok := subscription.GetID()
	if !ok {
		return fmt.Errorf("subscription has no id")
	}

	clusterConsole, ok := cluster.GetConsole()
	if !ok {
		return fmt.Errorf("cluster has no console url")
	}

	clusterURL, ok := clusterConsole.GetURL()
	if !ok {
		return fmt.Errorf("cluster has no console url")
	}

	displayName, ok := subscription.GetDisplayName()
	if !ok {
		return fmt.Errorf("subscription has no displayName")
	}

	ok = validateOldOwner(o, subscription, oldOwnerAccount)
	if !ok {
		fmt.Print("can't validate this is old owners cluster, this could be because of a previously failed run\n")
		err = utils.ConfirmSend()
		if err != nil {
			return err
		}
	}

	orgChanged := o.oldOrganizationId != o.newOrganizationId

	subscriptionOrgPatch, err := amv1.NewSubscription().OrganizationID(o.newOrganizationId).Build()

	if err != nil {
		return fmt.Errorf("can't create subscription organization patch: %w", err)
	}

	subscriptionCreatorPatchRequest, err := createSubscriptionCreatorPatchRequest(ocm, subscriptionID, accountID)

	if err != nil {
		return fmt.Errorf("can't create subscription creator patch: %w", err)
	}

	newRoleBinding, err := amv1.
		NewRoleBinding().
		AccountID(accountID).
		SubscriptionID(subscriptionID).
		Type("Subscription").
		RoleID("ClusterOwner").
		Build()

	if err != nil {
		return fmt.Errorf("can't create new owners rolebinding %w", err)
	}

	fmt.Printf("Transfer cluster: \t\t'%v' (%v)\n", externalClusterID, cluster.Name())
	fmt.Printf("from user \t\t\t'%v' to '%v'\n", oldOwnerAccount.ID(), accountID)
	if orgChanged {
		fmt.Printf("with organization change from \t'%v' to '%v'\n", o.oldOrganizationId, o.newOrganizationId)
	}

	if o.dryrun {
		fmt.Print("This is a dry run, nothing changed.\n")
		return nil
	}

	// Validation done, now update everything

	// org has to be patched before creator
	if orgChanged {
		subscriptionClient := ocm.AccountsMgmt().V1().Subscriptions().Subscription(subscriptionID)
		response, err := subscriptionClient.Update().Body(subscriptionOrgPatch).Send()

		if err != nil || response.Status() != 200 {
			return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
		}
		fmt.Printf("Patched organization on subscription\n")
	}

	// patch creator on subscription
	patchRes, err := subscriptionCreatorPatchRequest.Send()

	if err != nil || patchRes.Status() != 200 {
		return fmt.Errorf("request failed with status: %d, '%w'", patchRes.Status(), err)
	}
	fmt.Printf("Patched creator on subscription\n")

	// delete old rolebinding but do not exit on fail could be a rerun
	err = deleteOldRoleBinding(ocm, subscriptionID)

	if err != nil {
		fmt.Printf("can' delete old rolebinding %v \n", err)
	}

	// create new rolebinding
	newRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings()
	postRes, err := newRoleBindingClient.Add().Body(newRoleBinding).Send()

	// dont fail if the rolebinding already exists, could be rerun
	if err != nil {
		return fmt.Errorf("request failed '%w'", err)
	} else if postRes.Status() == 201 {
		fmt.Printf("Created new role binding.\n")
	} else if postRes.Status() == 409 {
		fmt.Printf("can't add new rolebinding, rolebinding already exists\n")
	} else {
		return fmt.Errorf("request failed with status: %d, '%w'", postRes.Status(), err)
	}

	// If the organization id has changed, re-register the cluster with CS with the new organization id
	if orgChanged {

		request, err := createNewRegisterClusterRequest(ocm, externalClusterID, subscriptionID, o.newOrganizationId, clusterURL, displayName)
		if err != nil {
			return fmt.Errorf("can't create RegisterClusterRequest with CS, '%w'", err)
		}

		response, err := request.Send()
		if err != nil || (response.Status() != 200 && response.Status() != 201) {
			return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
		}
		fmt.Print("Re-registered cluster\n")
	}

	err = validateTransfer(ocm, subscription.ClusterID(), o.newOrganizationId)
	if err != nil {
		return fmt.Errorf("error while validating transfer %w", err)
	}
	fmt.Print("Transfer complete\n")
	return nil
}

func getRoleBinding(ocm *sdk.Connection, subscriptionID string) (*amv1.RoleBinding, error) {
	roleBindingQuery := "subscription_id = '%s' and role_id = 'ClusterOwner'"
	searchString := fmt.Sprintf(roleBindingQuery, subscriptionID)
	response, err := ocm.AccountsMgmt().V1().RoleBindings().List().Parameter("search", searchString).
		Send()

	if err != nil {
		return nil, fmt.Errorf("can't send request: %v", err)
	}

	if response.Total() == 0 {
		return nil, fmt.Errorf("no rolebinding found")
	}

	_, ok := response.Items().Get(0).GetID()

	if !ok {
		return nil, fmt.Errorf("rolebinding has no ID")
	}

	return response.Items().Get(0), nil
}

// deletes old rolebinding by subscription id
func deleteOldRoleBinding(ocm *sdk.Connection, subscriptionID string) error {
	oldRoleBinding, err := getRoleBinding(ocm, subscriptionID)

	if err != nil {
		return fmt.Errorf("can't get old owners rolebinding %w", err)
	}

	oldRoleBindingID, ok := oldRoleBinding.GetID()
	if !ok {
		return fmt.Errorf("old rolebinding has no id %w", err)
	}
	oldRoleBindingClient := ocm.AccountsMgmt().V1().RoleBindings().RoleBinding(oldRoleBindingID)

	response, err := oldRoleBindingClient.Delete().Send()

	if err != nil {
		return fmt.Errorf("request failed '%w'", err)
	}
	if response.Status() == 204 {
		fmt.Printf("Deleted old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	if response.Status() == 404 {
		fmt.Printf("can't find old rolebinding: %v\n", oldRoleBindingID)
		return nil
	}
	fmt.Printf("request failed with status: %d\n", response.Status())
	return nil
}

type CreatorPatch struct {
	Creator_ID string `json:"creator_id"`
}

// creates subscription patch with new owner id
func createSubscriptionCreatorPatchRequest(ocm *sdk.Connection, subscriptionID string, accountID string) (*sdk.Request, error) {

	targetAPIPath := "/api/accounts_mgmt/v1/subscriptions/%s"

	request := ocm.Patch()
	err := arguments.ApplyPathArg(request, fmt.Sprintf(targetAPIPath, subscriptionID))
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %w", targetAPIPath, err)
	}

	body, err := json.Marshal(CreatorPatch{accountID})
	if err != nil {
		return nil, fmt.Errorf("cannot create body for request '%w'", err)
	}

	request.Bytes(body)

	return request, nil
}

type RegisterCluster struct {
	External_id     string `json:"external_id"`
	Subscription_id string `json:"subscription_id"`
	Organization_id string `json:"organization_id"`
	Console_url     string `json:"console_url"`
	Display_name    string `json:"display_name"`
}

// api schema 'ClusterRegistration' accepts two more optional fields that are not documented
// look at https://gitlab.cee.redhat.com/service/uhc-clusters-service/-/blob/master/pkg/mappers/inbound/clusters.go#L30
// fix is tracked in https://issues.redhat.com/browse/SDA-6652
// after ocm-sdk is fixed this function can be cleaned up

//re-register the cluster with CS with the new organization id
func createNewRegisterClusterRequest(ocm *sdk.Connection, externalClusterID string, subscriptionID string, organizationID string, consoleURL string, displayName string) (*sdk.Request, error) {

	targetAPIPath := "/api/clusters_mgmt/v1/register_cluster"

	request := ocm.Post()
	err := arguments.ApplyPathArg(request, targetAPIPath)
	if err != nil {
		return nil, fmt.Errorf("cannot parse API path '%s': %w", targetAPIPath, err)
	}

	body, err := json.Marshal(RegisterCluster{externalClusterID, subscriptionID, organizationID, consoleURL, displayName})
	if err != nil {
		return nil, fmt.Errorf("cannot create body for request '%w'", err)
	}

	request.Bytes(body)

	return request, nil
}

// Confirm the cluster record matches the subscription record regarding organization
func validateTransfer(ocm *sdk.Connection, clusterID string, newOrgID string) error {

	const query = "organization.id = '%s' and id = '%s'"
	searchString := fmt.Sprintf(query, newOrgID, clusterID)
	response, err := ocm.ClustersMgmt().V1().Clusters().List().Parameter("search", searchString).
		Send()

	if err != nil || response.Status() != 200 {
		return fmt.Errorf("request failed with status: %d, '%w'", response.Status(), err)
	}

	if response.Total() == 0 {
		return fmt.Errorf("organization id differs between cluster and subscription record")
	}

	return nil
}

// checks if old owner is on subscription
func validateOldOwner(o *transferOwnerOptions, subscription *amv1.Subscription, oldOwner *amv1.Account) bool {
	oldOrgSub, ok := subscription.GetOrganizationID()
	if !ok {
		fmt.Printf("subscription has no organization\n")
		return ok
	}
	oldOwnerSub, ok := subscription.GetCreator()
	if !ok {
		fmt.Printf("subscription has no creator\n")
		return ok
	}
	userIDSub, ok := oldOwnerSub.GetID()
	if !ok {
		fmt.Printf("old owner has no id\n")
		return ok
	}

	ok = true
	if oldOrgSub != o.oldOrganizationId {
		fmt.Printf("old owners organization on subscription differs from the specified one. ")
		fmt.Printf("Subscription has organization ID: %v (specified was: %v)\n", oldOrgSub, o.oldOrganizationId)
		ok = false
	}
	if userIDSub != oldOwner.ID() {
		fmt.Printf("old owners ID on subscription differs from the specified one. ")
		fmt.Printf("Subscription has owner: %v (specified was: %v)\n", userIDSub, oldOwner.ID())
		ok = false
	}
	return ok
}
