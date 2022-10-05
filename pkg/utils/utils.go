package utils

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type LimitedSupportReasonItem struct {
	ID      string
	Summary string
	Details string
}

var clusterKeyRE = regexp.MustCompile(`^(\w|-)+$`)

func IsValidKey(clusterKey string) bool {
	return clusterKeyRE.MatchString(clusterKey)
}

func IsValidClusterKey(clusterKey string) (err error) {
	if !IsValidKey(clusterKey) {
		return fmt.Errorf(
			"Cluster name, identifier or external identifier '%s' isn't valid: it "+
				"must contain only letters, digits, dashes and underscores",
			clusterKey,
		)
	}
	return nil
}

// GetCluster Function allows to get a single cluster with any identifier (displayname, ID, or external ID)
func GetCluster(connection *sdk.Connection, key string) (cluster *cmv1.Cluster, err error) {
	// Prepare the resources that we will be using:
	subsResource := connection.AccountsMgmt().V1().Subscriptions()
	clustersResource := connection.ClustersMgmt().V1().Clusters()

	// Try to find a matching subscription:
	subsSearch := fmt.Sprintf(
		"(display_name = '%s' or cluster_id = '%s' or external_cluster_id = '%s') and "+
			"status in ('Reserved', 'Active')",
		key, key, key,
	)
	subsListResponse, err := subsResource.List().
		Search(subsSearch).
		Size(1).
		Send()
	if err != nil {
		err = fmt.Errorf("Can't retrieve subscription for key '%s': %v", key, err)
		return
	}

	// If there is exactly one matching subscription then return the corresponding cluster:
	subsTotal := subsListResponse.Total()
	if subsTotal == 1 {
		id, ok := subsListResponse.Items().Slice()[0].GetClusterID()
		if ok {
			var clusterGetResponse *cmv1.ClusterGetResponse
			clusterGetResponse, err = clustersResource.Cluster(id).Get().
				Send()
			if err != nil {
				err = fmt.Errorf(
					"Can't retrieve cluster for key '%s': %v",
					key, err,
				)
				return
			}
			cluster = clusterGetResponse.Body()
			return
		}
	}

	// If there are multiple subscriptions that match the cluster then we should report it as
	// an error:
	if subsTotal > 1 {
		err = fmt.Errorf(
			"There are %d subscriptions with cluster identifier or name '%s'",
			subsTotal, key,
		)
		return
	}

	// If we are here then no subscription matches the passed key. It may still be possible that
	// the cluster exists but it is not reporting metrics, so it will not have the external
	// identifier in the accounts management service. To find those clusters we need to check
	// directly in the clusters management service.
	clustersSearch := fmt.Sprintf(
		"id = '%s' or name = '%s' or external_id = '%s'",
		key, key, key,
	)
	clustersListResponse, err := clustersResource.List().
		Search(clustersSearch).
		Size(1).
		Send()
	if err != nil {
		err = fmt.Errorf("Can't retrieve clusters for key '%s': %v", key, err)
		return
	}

	// If there is exactly one cluster matching then return it:
	clustersTotal := clustersListResponse.Total()
	if clustersTotal == 1 {
		cluster = clustersListResponse.Items().Slice()[0]
		return
	}

	// If there are multiple matching clusters then we should report it as an error:
	if clustersTotal > 1 {
		err = fmt.Errorf(
			"There are %d clusters with identifier or name '%s'",
			clustersTotal, key,
		)
		return
	}

	// If we are here then there are no subscriptions or clusters matching the passed key:
	err = fmt.Errorf(
		"There are no subscriptions or clusters with identifier or name '%s'",
		key,
	)
	return
}

func GetClusterLimitedSupportReasons(connection *sdk.Connection, clusterID string) ([]*LimitedSupportReasonItem, error) {

	limitedSupportReasons, err := connection.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterID).
		LimitedSupportReasons().
		List().
		Send()
	if err != nil {
		return nil, fmt.Errorf("Failed to get limited Support Reasons: %s", err)
	}

	lmtReason := limitedSupportReasons.Items().Slice()

	var clusterLmtSprReasons []*LimitedSupportReasonItem

	for _, reason := range lmtReason {
		clusterLmtSprReason := LimitedSupportReasonItem{
			ID:      reason.ID(),
			Summary: reason.Summary(),
			Details: reason.Details(),
		}
		clusterLmtSprReasons = append(clusterLmtSprReasons, &clusterLmtSprReason)
	}

	return clusterLmtSprReasons, nil
}

// GetSubscription Function allows to get a single subscription with any identifier (displayname, ID, internal or external ID)
func GetSubscription(connection *sdk.Connection, key string) (subscription *amv1.Subscription, err error) {
	// Prepare the resources that we will be using:
	subsResource := connection.AccountsMgmt().V1().Subscriptions()

	// Try to find a matching subscription:
	subsSearch := fmt.Sprintf(
		"(display_name = '%s' or cluster_id = '%s' or external_cluster_id = '%s' or id = '%s')",
		key, key, key, key)
	subsListResponse, err := subsResource.List().Parameter("search", subsSearch).Send()
	if err != nil {
		err = fmt.Errorf("can't retrieve subscription for key '%s': %v", key, err)
		return
	}

	// If there is exactly one matching subscription then return the corresponding cluster:
	subsTotal := subsListResponse.Total()
	if subsTotal == 1 {
		return subsListResponse.Items().Get(0), nil
	}

	// If there are multiple subscriptions that match the key then we should report it as
	// an error:
	if subsTotal > 1 {
		err = fmt.Errorf(
			"there are %d subscriptions with cluster identifier or name '%s'",
			subsTotal, key,
		)
		return
	}
	// If we are here then there are no subscriptions matching the passed key:
	err = fmt.Errorf(
		"there are no subscriptions with identifier or name '%s'",
		key,
	)
	return
}

// GetAccount Function allows to get a single account with any identifier (username, ID)
func GetAccount(connection *sdk.Connection, key string) (account *amv1.Account, err error) {
	// Prepare the resources that we will be using:
	accsResource := connection.AccountsMgmt().V1().Accounts()

	// Try to find a matching account:
	search := fmt.Sprintf("(username = '%s' or id = '%s')", key, key)
	accsListResponse, err := accsResource.List().Parameter("search", search).Send()
	if err != nil {
		err = fmt.Errorf("can't retrieve account for key '%s': %v", key, err)
		return
	}

	// If there is exactly one matching account then return it:
	accsTotal := accsListResponse.Total()
	if accsTotal == 1 {
		return accsListResponse.Items().Get(0), nil
	}

	// If there are multiple accounts that match the key then we should report it as
	// an error:
	if accsTotal > 1 {
		err = fmt.Errorf(
			"there are %d accounts with id or username '%s'",
			accsTotal, key,
		)
		return
	}
	// If we are here then there are no accounts matching the passed key:
	err = fmt.Errorf(
		"there are no accounts with identifier or username '%s'",
		key,
	)
	return
}

func ConfirmSend() error {
	fmt.Print("Continue? (y/N): ")

	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		return err
	}

	switch strings.ToLower(response) {
	case "y", "yes":
		return nil
	case "n", "no":
		return fmt.Errorf("Exiting...")
	default:
		fmt.Println("Invalid input. Expecting (y)es or (N)o")
		return ConfirmSend()
	}
}

// streamPrintln appends a newline then prints the given msg using the provided IOStreams
func StreamPrintln(stream genericclioptions.IOStreams, msg string) {
	stream.Out.Write([]byte(fmt.Sprintln(msg)))
}

// streamPrint prints the given msg using the provided IOStreams
func StreamPrint(stream genericclioptions.IOStreams, msg string) {
	stream.Out.Write([]byte(msg))
}

// streamPrint prints the given error msg using the provided IOStreams
func StreamErrorln(stream genericclioptions.IOStreams, msg string) {
	stream.ErrOut.Write([]byte(fmt.Sprintln(msg)))
}

// StreamRead retrieves input from the provided IOStreams up to (and including) the delimiter given
func StreamRead(stream genericclioptions.IOStreams, delim byte) (string, error) {
	reader := bufio.NewReader(stream.In)
	return reader.ReadString(delim)
}

// Contains returns true if the given key is present in the provided list
func Contains(list []string, key string) bool {
	for _, item := range list {
		if item == key {
			return true
		}
	}
	return false
}
