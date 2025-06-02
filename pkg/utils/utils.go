package utils

import (
	"bufio"
	"errors"
	"fmt"
	"regexp"
	"runtime/debug"
	"strings"

	"github.com/andygrunwald/go-jira"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

type fieldQuery struct {
	Field    string
	Operator string
	Value    string
}

const (
	JiraHandoverAnnouncementProjectKey = "SRE Platform HandOver Announcements"
	JiraBaseURL                        = "https://issues.redhat.com"
	productCustomField                 = "customfield_12319040"
	customerNameCustomField            = "customfield_12310160"
)

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

func GetCurrentOCMEnv(connection *sdk.Connection) string {

	// Default to production
	currentEnv := "production"

	url := connection.URL()
	if strings.Contains(url, "stage") {
		currentEnv = "stage"
	}
	if strings.Contains(url, "integration") {
		currentEnv = "integration"
	}
	return currentEnv
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

func GetClusterLimitedSupportReasons(connection *sdk.Connection, clusterID string) ([]*cmv1.LimitedSupportReason, error) {
	limitedSupportReasons, err := connection.ClustersMgmt().V1().
		Clusters().
		Cluster(clusterID).
		LimitedSupportReasons().
		List().
		Send()
	if err != nil {
		return nil, fmt.Errorf("Failed to get limited Support Reasons: %s", err)
	}

	return limitedSupportReasons.Items().Slice(), nil
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

// GetOrganization returns an *amv1.Organization given an OCM cluster name, external id, or internal id as key
func GetOrganization(connection *sdk.Connection, key string) (*amv1.Organization, error) {
	subscription, err := GetSubscription(connection, key)
	if err != nil {
		return nil, err
	}
	orgResponse, err := connection.AccountsMgmt().V1().Organizations().Organization(subscription.OrganizationID()).Get().Send()
	if err != nil {
		return nil, err
	}
	return orgResponse.Body(), nil
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

func GetRegistryCredentials(connection *sdk.Connection, accountId string) ([]*amv1.RegistryCredential, error) {
	searchString := fmt.Sprintf("account_id = '%s'", accountId)
	registryCredentials, err := connection.AccountsMgmt().V1().RegistryCredentials().List().Search(searchString).Send()
	if err != nil {
		return nil, err
	}
	return registryCredentials.Items().Slice(), nil
}

func ConfirmPrompt() bool {
	fmt.Print("Continue? (y/N): ")

	var response = "n"
	_, _ = fmt.Scanln(&response) // Erroneous input will be handled by the default case below

	switch strings.ToLower(response) {
	case "y", "yes":
		return true
	case "n", "no":
		return false
	default:
		fmt.Println("Invalid input. Expecting (y)es or (N)o")
		return ConfirmPrompt()
	}
}

// StreamPrintln appends a newline then prints the given msg using the provided IOStreams
func StreamPrintln(stream genericclioptions.IOStreams, msg string) {
	stream.Out.Write([]byte(fmt.Sprintln(msg)))
}

// StreamPrint prints the given msg using the provided IOStreams
func StreamPrint(stream genericclioptions.IOStreams, msg string) {
	stream.Out.Write([]byte(msg))
}

// StreamErrorln prints the given error msg using the provided IOStreams
func StreamErrorln(stream genericclioptions.IOStreams, msg string) {
	stream.ErrOut.Write([]byte(fmt.Sprintln(msg)))
}

// StreamRead retrieves input from the provided IOStreams up to (and including) the delimiter given
func StreamRead(stream genericclioptions.IOStreams, delim byte) (string, error) {
	reader := bufio.NewReader(stream.In)
	return reader.ReadString(delim)
}

var ReadBuildInfo = debug.ReadBuildInfo

func GetDependencyVersion(dependencyPath string) (string, error) {
	buildInfo, ok := ReadBuildInfo()
	if !ok {
		return "", errors.New("failed to parse build info")
	}
	for _, dep := range buildInfo.Deps {
		if dep.Path == dependencyPath {
			return dep.Version, nil
		}
	}

	return "", fmt.Errorf("unable to find version for %v", dependencyPath)
}

func determineClusterProduct(productID string, isHCP bool) (productName string) {
	if productID == "rosa" && isHCP {
		productName = "Red Hat OpenShift on AWS with Hosted Control Planes"
	} else if productID == "rosa" {
		productName = "Red Hat OpenShift on AWS"
	} else if productID == "osd" {
		productName = "OpenShift Dedicated"
	}
	return productName
}
func buildJQL(projectKey string, filters []fieldQuery) string {
	var conditions []string
	for _, q := range filters {
		switch q.Operator {
		case "~*":
			values := strings.Split(q.Value, ",")
			var orParts []string
			for _, v := range values {
				orParts = append(orParts,
					fmt.Sprintf(`(project = "%s" AND "%s" ~ "%s")`, projectKey, q.Field, strings.TrimSpace(v)))
			}
			conditions = append(conditions, "("+strings.Join(orParts, " OR ")+")")

		case "in":
			conditions = append(conditions,
				fmt.Sprintf(`(project = "%s" AND "%s" in (%s))`, projectKey, q.Field, q.Value),
			)

		default:
			conditions = append(conditions,
				fmt.Sprintf(`(project = "%s" AND "%s" %s "%s")`, projectKey, q.Field, q.Operator, q.Value),
			)
		}
	}
	return "(" + strings.Join(conditions, " OR ") + ") AND status != Closed ORDER BY created DESC"
}

func formatVersion(version string) string {
	versionParts := strings.Split(version, ".")
	versionPrefix := version
	if len(versionParts) >= 2 {
		versionPrefix = fmt.Sprintf("%s.%s", versionParts[0], versionParts[1])
	}
	return versionPrefix
}

func isValidMatch(i jira.Issue, orgName string, product string, version string) bool {
	isIgnored := func(val string) bool {
		val = strings.ToLower(strings.TrimSpace(val))
		return val == "none" || val == "n/a" || val == "all" || val == ""
	}

	hasMatchingValue := func(items []interface{}, expected string) bool {
		expected = strings.ToLower(strings.TrimSpace(expected))
		for _, item := range items {
			if m, ok := item.(map[string]interface{}); ok {
				if val, ok := m["value"].(string); ok {
					val = strings.ToLower(strings.TrimSpace(val))
					if val == expected {
						return true
					}
				}
			}
		}
		return false
	}

	productRaw := i.Fields.Unknowns[productCustomField]
	versionRaw := i.Fields.AffectsVersions
	nameRaw := i.Fields.Unknowns[customerNameCustomField]

	productMatch := false
	if items, ok := productRaw.([]interface{}); ok {
		productMatch = hasMatchingValue(items, product)
	}
	if !productMatch {
		return false
	}

	versionMatch := false
	clusterFormattedVersion := formatVersion(version)

	for _, v := range versionRaw {
		if v != nil {
			vFormatted := formatVersion(v.Name)
			if vFormatted == clusterFormattedVersion || isIgnored(v.Name) {
				versionMatch = true
				break
			}
		}
	}

	nameMatch := false
	if nameStr, ok := nameRaw.(string); ok {
		parts := strings.Split(nameStr, ";")
		for _, part := range parts {
			val := strings.TrimSpace(part)
			if val == orgName || isIgnored(val) {
				nameMatch = true
				break
			}
		}
	}

	return versionMatch || (nameMatch && versionMatch)
}
