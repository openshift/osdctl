package controller

import (
	"bufio"
	"context"
	b64 "encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"time"

	"math/rand"
	"strings"

	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	hiveapiv1 "github.com/openshift/hive/apis/hive/v1"
	hiveinternalv1alpha1 "github.com/openshift/hive/apis/hiveinternal/v1alpha1"
	hypershiftv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/openshift/osdctl/pkg/utils"
)

var (
	psColorOK   = color.New(color.FgGreen).SprintFunc()
	psColorWarn = color.New(color.FgYellow).SprintFunc()
)

// RequiredPullSecretAuths lists the registry auth entries that must be present
// in a cluster's pull secret for the cluster to function.
var RequiredPullSecretAuths = []string{
	"cloud.openshift.com",
	"quay.io",
	"registry.redhat.io",
	"registry.connect.redhat.com",
}

// ClusterSummary holds subscription-level data for a cluster owned by an account.
type ClusterSummary struct {
	Name      string
	ID        string
	Status    string
	CreatedAt time.Time
}

// AuthCheckResult holds the outcome of a single registry auth comparison.
type AuthCheckResult struct {
	Registry   string
	Source     string // "access_token" or "registry_credential"
	OK         bool
	TokenMatch bool
	EmailMatch bool
	Email      string
	Detail     string
}

// PullSecretVerifyResult holds the outcome of a per-registry auth comparison.
type PullSecretVerifyResult struct {
	Matched         int
	Total           int
	Mismatches      []string
	AuthResults     []AuthCheckResult
	MissingRequired []string
}

// FetchOwnerAccessToken retrieves the cluster owner's pull secret from OCM,
// using impersonation if the current OCM user is not the cluster owner.
// Returns the marshaled pull secret bytes and the raw auth map for verification.
func FetchOwnerAccessToken(ocm *sdk.Connection, ownerUsername string, logger *logrus.Logger) ([]byte, map[string]*amv1.AccessTokenAuth, error) {
	currentAccountResp, err := ocm.AccountsMgmt().V1().CurrentAccount().Get().Send()
	if err != nil {
		logger.Warnf("Could not fetch current account info, will use impersonation: %v", err)
	}

	var response *amv1.AccessTokenPostResponse
	if currentAccountResp != nil && currentAccountResp.Body().Username() == ownerUsername {
		logger.Info("Current OCM user matches cluster owner, fetching access token directly")
		response, err = ocm.AccountsMgmt().V1().AccessToken().Post().Send()
	} else {
		logger.Infof("Impersonating cluster owner '%s' to fetch access token", ownerUsername)
		response, err = ocm.AccountsMgmt().V1().AccessToken().Post().Impersonate(ownerUsername).Parameter("body", nil).Send()
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch OCM access token: %w", err)
	}

	auths, ok := response.Body().GetAuths()
	if !ok {
		return nil, nil, fmt.Errorf("failed to get auths from access token response — contact SDB if this persists")
	}

	authsMap := map[string]map[string]string{}
	for k, auth := range auths {
		authsMap[k] = map[string]string{
			"auth":  auth.Auth(),
			"email": auth.Email(),
		}
	}

	pullSecret, err := json.Marshal(map[string]map[string]map[string]string{
		"auths": authsMap,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal pull secret: %w", err)
	}

	return pullSecret, auths, nil
}

// ValidateRequiredAuths checks that the OCM access token includes all required
// registry auth entries. Returns the list of missing registries.
func ValidateRequiredAuths(auths map[string]*amv1.AccessTokenAuth) []string {
	var missing []string
	for _, required := range RequiredPullSecretAuths {
		if _, ok := auths[required]; !ok {
			missing = append(missing, required)
		}
	}
	return missing
}

// CompareAccessTokenAuthsToCluster compares OCM access token auths against the pull
// secret on the target cluster. Writes per-registry results to out.
// Returns a PullSecretVerifyResult with match counts and any mismatches.
func CompareAccessTokenAuthsToCluster(ctx context.Context, clientset *kubernetes.Clientset, expectedAuths map[string]*amv1.AccessTokenAuth, out io.Writer) (*PullSecretVerifyResult, error) {
	pullSecret, err := clientset.CoreV1().Secrets("openshift-config").Get(ctx, "pull-secret", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret openshift-config/pull-secret from target cluster: %w", err)
	}

	if _, ok := pullSecret.Data[".dockerconfigjson"]; !ok {
		return nil, fmt.Errorf("secret openshift-config/pull-secret is missing .dockerconfigjson key")
	}

	result := &PullSecretVerifyResult{Total: len(expectedAuths)}

	sortedKeys := make([]string, 0, len(expectedAuths))
	for k := range expectedAuths {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, authKey := range sortedKeys {
		expectedAuth := expectedAuths[authKey]
		ar := AuthCheckResult{Registry: authKey, Source: "access_token"}

		clusterAuth, err := extractPullSecretAuth(authKey, pullSecret)
		if err != nil {
			result.Mismatches = append(result.Mismatches, authKey)
			ar.Detail = "not found in cluster secret"
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		ar.TokenMatch = clusterAuth.auth == expectedAuth.Auth()
		ar.EmailMatch = clusterAuth.email == expectedAuth.Email()
		ar.Email = expectedAuth.Email()

		if !ar.TokenMatch || !ar.EmailMatch {
			result.Mismatches = append(result.Mismatches, authKey)
			details := ""
			if !ar.TokenMatch {
				details += "token mismatch"
			}
			if !ar.EmailMatch {
				if details != "" {
					details += ", "
				}
				details += fmt.Sprintf("email mismatch (cluster=%q, OCM=%q)", clusterAuth.email, expectedAuth.Email())
			}
			ar.Detail = details
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		ar.OK = true
		result.Matched++
		result.AuthResults = append(result.AuthResults, ar)
	}

	for _, required := range RequiredPullSecretAuths {
		if _, err := extractPullSecretAuth(required, pullSecret); err != nil {
			result.MissingRequired = append(result.MissingRequired, required)
		}
	}

	// Write human-readable output if a writer is provided
	if out != nil {
		RenderVerifyResult(result, out)
	}

	return result, nil
}

// RenderVerifyResult writes the verification result in human-readable format.
func RenderVerifyResult(result *PullSecretVerifyResult, out io.Writer) {
	mismatchLine := color.New(color.FgYellow, color.Bold).SprintFunc()
	for _, ar := range result.AuthResults {
		if ar.OK {
			fmt.Fprintf(out, "  %s %-40s token=match, email=match (%s)\n", psColorOK("[OK]"), ar.Registry, ar.Email)
		} else {
			fmt.Fprintf(out, "  %s\n", mismatchLine(fmt.Sprintf("[!] %-40s %s", ar.Registry, ar.Detail)))
		}
	}

	fmt.Fprintf(out, "\n  Verified %d/%d auth entries match\n", result.Matched, result.Total)

	if len(result.MissingRequired) > 0 {
		fmt.Fprintf(out, "\n%s cluster pull secret is missing required registries:\n", psColorWarn("[WARN]"))
		for _, m := range result.MissingRequired {
			fmt.Fprintf(out, "  - %s\n", m)
		}
		fmt.Fprintf(out, "The cluster may have issues pulling images or reporting telemetry.\n")
	} else {
		fmt.Fprintf(out, "  %s All required registries present in cluster pull secret\n", psColorOK("[OK]"))
	}
}

// CompareRegistryCredentialAuthsToCluster compares OCM registry credentials against the pull
// secret on the target cluster. Registry credentials use a different token
// format (base64-encoded "username:token") than access token auths.
func CompareRegistryCredentialAuthsToCluster(ctx context.Context, ocm *sdk.Connection, clientset *kubernetes.Clientset, accountID string, accountEmail string, out io.Writer) (*PullSecretVerifyResult, error) {
	creds, err := utils.GetRegistryCredentials(ocm, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch registry credentials: %w", err)
	}
	if len(creds) == 0 {
		return nil, fmt.Errorf("no registry credentials found for account %s", accountID)
	}

	pullSecret, err := clientset.CoreV1().Secrets("openshift-config").Get(ctx, "pull-secret", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret openshift-config/pull-secret: %w", err)
	}

	result := &PullSecretVerifyResult{Total: len(creds)}

	for _, cred := range creds {
		registryID := cred.Registry().ID()

		// Resolve registry name from OCM
		regResp, err := ocm.AccountsMgmt().V1().Registries().Registry(registryID).Get().Send()
		if err != nil {
			ar := AuthCheckResult{Registry: registryID, Source: "registry_credential", Detail: fmt.Sprintf("cannot resolve registry: %v", err)}
			result.Mismatches = append(result.Mismatches, registryID)
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}
		regName, _ := regResp.Body().GetName()
		if regName == "" {
			regName = registryID
		}

		ar := AuthCheckResult{Registry: regName, Source: "registry_credential"}

		token, _ := cred.GetToken()
		username, _ := cred.GetUsername()
		if token == "" || username == "" {
			ar.Detail = "missing token or username in OCM registry credential"
			result.Mismatches = append(result.Mismatches, regName)
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		clusterAuth, err := extractPullSecretAuth(regName, pullSecret)
		if err != nil {
			ar.Detail = "not found in cluster secret"
			result.Mismatches = append(result.Mismatches, regName)
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		// Registry credential tokens are stored as base64("username:token") in the cluster secret
		expectedToken := fmt.Sprintf("%s:%s", username, token)
		clusterTokenDecoded, err := b64.StdEncoding.DecodeString(clusterAuth.auth)
		if err != nil {
			ar.Detail = "failed to decode cluster token"
			result.Mismatches = append(result.Mismatches, regName)
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		ar.TokenMatch = expectedToken == string(clusterTokenDecoded)
		ar.EmailMatch = accountEmail == clusterAuth.email
		ar.Email = accountEmail

		if !ar.TokenMatch || !ar.EmailMatch {
			result.Mismatches = append(result.Mismatches, regName)
			details := ""
			if !ar.TokenMatch {
				details += "token mismatch"
			}
			if !ar.EmailMatch {
				if details != "" {
					details += ", "
				}
				details += fmt.Sprintf("email mismatch (cluster=%q, OCM=%q)", clusterAuth.email, accountEmail)
			}
			ar.Detail = details
			result.AuthResults = append(result.AuthResults, ar)
			continue
		}

		ar.OK = true
		result.Matched++
		result.AuthResults = append(result.AuthResults, ar)
	}

	if out != nil {
		RenderVerifyResult(result, out)
	}

	return result, nil
}

// ThreeWayAuthState describes the sync state of a single auth entry across OCM, hive, and target.
type ThreeWayAuthState struct {
	Registry          string
	InOCM             bool
	InHive            bool
	InTarget          bool
	OCMMatchesHive    bool
	OCMMatchesTarget  bool
	HiveMatchesTarget bool
}

// ThreeWayComparison holds the full comparison result across OCM, hive, and target.
type ThreeWayComparison struct {
	Auths           []ThreeWayAuthState
	HiveNeedsUpdate bool
	TargetNeedsSync bool
	AllInSync       bool
}

// SimpleAuth holds a registry auth's token and email for generic comparison.
type SimpleAuth struct {
	Auth  string
	Email string
}

// AccessTokenToSimple converts access token auths to SimpleAuth map.
func AccessTokenToSimple(auths map[string]*amv1.AccessTokenAuth) map[string]SimpleAuth {
	result := make(map[string]SimpleAuth, len(auths))
	for k, v := range auths {
		result[k] = SimpleAuth{Auth: v.Auth(), Email: v.Email()}
	}
	return result
}

// CompareThreeWay compares pull secret auths across OCM, hive secret, and target cluster secret.
// ocmAuths maps registry name → SimpleAuth with the expected auth/email values.
// hiveData and targetData are the raw .dockerconfigjson bytes from each secret.
func CompareThreeWay(ocmAuths map[string]SimpleAuth, hiveData []byte, targetData []byte) (*ThreeWayComparison, error) {
	result := &ThreeWayComparison{AllInSync: true}

	type parsedAuth struct {
		Auth  string `json:"auth"`
		Email string `json:"email"`
	}
	type parsedPS struct {
		Auths map[string]parsedAuth `json:"auths"`
	}

	var hive, target parsedPS
	hiveAuths := make(map[string]parsedAuth)
	targetAuths := make(map[string]parsedAuth)

	if len(hiveData) > 0 {
		if err := json.Unmarshal(hiveData, &hive); err != nil {
			return nil, fmt.Errorf("failed to parse hive pull secret: %w", err)
		}
		hiveAuths = hive.Auths
	}
	if len(targetData) > 0 {
		if err := json.Unmarshal(targetData, &target); err != nil {
			return nil, fmt.Errorf("failed to parse target pull secret: %w", err)
		}
		targetAuths = target.Auths
	}

	// Only compare registries present in the OCM source being checked.
	// Registries in hive/target but not in OCM are outside this source's scope.
	for registry := range ocmAuths {
		state := ThreeWayAuthState{Registry: registry}

		ocmAuth, inOCM := ocmAuths[registry]
		hiveAuth, inHive := hiveAuths[registry]
		targetAuth, inTarget := targetAuths[registry]

		state.InOCM = inOCM
		state.InHive = inHive
		state.InTarget = inTarget

		if inOCM && inHive {
			state.OCMMatchesHive = ocmAuth.Auth == hiveAuth.Auth && ocmAuth.Email == hiveAuth.Email
		}
		if inOCM && inTarget {
			state.OCMMatchesTarget = ocmAuth.Auth == targetAuth.Auth && ocmAuth.Email == targetAuth.Email
		}
		if inHive && inTarget {
			state.HiveMatchesTarget = hiveAuth.Auth == targetAuth.Auth && hiveAuth.Email == targetAuth.Email
		}

		// Determine if sync is needed
		if inOCM && !state.OCMMatchesHive {
			result.HiveNeedsUpdate = true
			result.AllInSync = false
		}
		if inOCM && !state.OCMMatchesTarget {
			result.TargetNeedsSync = true
			result.AllInSync = false
		}
		if inHive && inTarget && !state.HiveMatchesTarget {
			result.AllInSync = false
		}

		result.Auths = append(result.Auths, state)
	}

	sort.Slice(result.Auths, func(i, j int) bool {
		return result.Auths[i].Registry < result.Auths[j].Registry
	})

	return result, nil
}

// RenderThreeWayComparison prints the three-way comparison in a readable format.
// sourceLabel identifies the OCM source (e.g. "ACCESS TOKEN AUTHS", "REGISTRY CREDENTIAL AUTHS").
// When hasHive is false (HCP clusters), the hive columns are omitted.
func RenderThreeWayComparison(result *ThreeWayComparison, sourceLabel string, hasHive bool, out io.Writer) {
	table := tablewriter.NewWriter(out)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetBorder(false)
	table.SetColumnSeparator("  ")
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(false)

	if hasHive {
		table.SetHeader([]string{sourceLabel, "OCM↔HIVE", "OCM↔TARGET", "HIVE↔TARGET"})
		table.SetHeaderColor(
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
		)
		for _, a := range result.Auths {
			table.Append([]string{
				a.Registry,
				syncStatus(a.InHive, a.OCMMatchesHive),
				syncStatus(a.InTarget, a.OCMMatchesTarget),
				syncStatus(a.InHive && a.InTarget, a.HiveMatchesTarget),
			})
		}
	} else {
		table.SetHeader([]string{sourceLabel, "OCM↔TARGET"})
		table.SetHeaderColor(
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
			tablewriter.Colors{tablewriter.Bold, tablewriter.FgBlueColor},
		)
		for _, a := range result.Auths {
			table.Append([]string{
				a.Registry,
				syncStatus(a.InTarget, a.OCMMatchesTarget),
			})
		}
	}
	table.Render()

	fmt.Fprintln(out)
	if result.AllInSync {
		fmt.Fprintf(out, "  %s All sources in sync\n", psColorOK("[OK]"))
	} else {
		if hasHive && result.HiveNeedsUpdate {
			fmt.Fprintf(out, "  %s Hive secret needs update from OCM\n", psColorWarn("[!]"))
		}
		if result.TargetNeedsSync {
			fmt.Fprintf(out, "  %s Target cluster needs update from OCM\n", psColorWarn("[!]"))
		}
	}
}

func syncStatus(present bool, matches bool) string {
	if !present {
		return color.New(color.FgYellow).Sprint("missing")
	}
	if matches {
		return color.New(color.FgGreen).Sprint("match")
	}
	return color.New(color.FgYellow, color.Bold).Sprint("DIFFERS")
}

// HiveNamespaceInfo holds the resolved Hive namespace and ClusterDeployment name
// for a given cluster.
type HiveNamespaceInfo struct {
	Namespace             string
	ClusterDeploymentName string
}

// FindHiveNamespace discovers the Hive namespace for a cluster by listing
// ClusterDeployments filtered by the api.openshift.com/id label. This avoids
// the fragile uhc-{env}-{clusterID} namespace construction.
func FindHiveNamespace(ctx context.Context, kubeCli client.Client, clusterID string) (*HiveNamespaceInfo, error) {
	if err := hiveapiv1.AddToScheme(kubeCli.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to add hive scheme: %w", err)
	}

	// Try label-based lookup first (fast, targeted)
	cdList := &hiveapiv1.ClusterDeploymentList{}
	labelSelector := client.MatchingLabels{"api.openshift.com/id": clusterID}
	if err := kubeCli.List(ctx, cdList, labelSelector); err == nil && len(cdList.Items) > 0 {
		cd := cdList.Items[0]
		return &HiveNamespaceInfo{
			Namespace:             cd.Namespace,
			ClusterDeploymentName: cd.Name,
		}, nil
	}

	// Fallback: list all ClusterDeployments and match by ClusterMetadata
	allCDs := &hiveapiv1.ClusterDeploymentList{}
	if err := kubeCli.List(ctx, allCDs); err != nil {
		return nil, fmt.Errorf("failed to list ClusterDeployments: %w", err)
	}

	for _, cd := range allCDs.Items {
		if cd.Spec.ClusterMetadata != nil && cd.Spec.ClusterMetadata.ClusterID == clusterID {
			return &HiveNamespaceInfo{
				Namespace:             cd.Namespace,
				ClusterDeploymentName: cd.Name,
			}, nil
		}
	}

	return nil, fmt.Errorf("no ClusterDeployment found for cluster ID %s", clusterID)
}

// CountOwnerClusters returns the number of active clusters owned by the given
// account ID.
func CountOwnerClusters(ocm *sdk.Connection, accountID string, logger *logrus.Logger) int {
	search := fmt.Sprintf("creator.id = '%s' and status != 'Deprovisioned' and status != 'Archived'", accountID)
	resp, err := ocm.AccountsMgmt().V1().Subscriptions().List().
		Search(search).
		Size(1).
		Send()
	if err != nil {
		logger.Debugf("Could not query sibling clusters: %v", err)
		return 0
	}
	return resp.Total()
}

// ListOwnerSubscriptions returns all active subscriptions for the given account ID.
func ListOwnerSubscriptions(ocm *sdk.Connection, accountID string) ([]ClusterSummary, error) {
	search := fmt.Sprintf("creator.id = '%s' and status != 'Deprovisioned' and status != 'Archived'", accountID)
	pageSize := 100
	request := ocm.AccountsMgmt().V1().Subscriptions().List().
		Search(search).
		Size(pageSize)

	var clusters []ClusterSummary
	for {
		resp, err := request.Send()
		if err != nil {
			return nil, err
		}

		for _, sub := range resp.Items().Slice() {
			name, _ := sub.GetDisplayName()
			clusterID, _ := sub.GetClusterID()
			status, _ := sub.GetStatus()
			createdAt, _ := sub.GetCreatedAt()

			if clusterID == "" {
				continue
			}

			clusters = append(clusters, ClusterSummary{
				Name:      name,
				ID:        clusterID,
				Status:    status,
				CreatedAt: createdAt,
			})
		}

		if resp.Size() < pageSize {
			break
		}
		request.Page(resp.Page() + 1)
	}

	return clusters, nil
}

// GetLatestCredentialUpdate returns the most recent UpdatedAt time across
// all registry credentials for the given account.
func GetLatestCredentialUpdate(ocm *sdk.Connection, accountID string) (time.Time, error) {
	creds, err := utils.GetRegistryCredentials(ocm, accountID)
	if err != nil {
		return time.Time{}, err
	}

	var latest time.Time
	for _, cred := range creds {
		if updated, ok := cred.GetUpdatedAt(); ok {
			if updated.After(latest) {
				latest = updated
			}
		}
	}
	return latest, nil
}

// pullSecretAuthEntry holds extracted auth data from a cluster pull secret.
type pullSecretAuthEntry struct {
	auth  string
	email string
}

// ExtractRegistryAuth extracts the "auth" field for a given registry from raw pull secret JSON bytes.
func ExtractRegistryAuth(pullSecretData []byte, registry string) (string, error) {
	var parsed struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(pullSecretData, &parsed); err != nil {
		return "", fmt.Errorf("failed to parse pull secret: %w", err)
	}
	entry, ok := parsed.Auths[registry]
	if !ok {
		return "", fmt.Errorf("registry %s not found in pull secret auths", registry)
	}
	return entry.Auth, nil
}

// extractPullSecretAuth extracts an auth entry from a cluster pull secret by registry name.
func extractPullSecretAuth(authID string, secret *corev1.Secret) (*pullSecretAuthEntry, error) {
	dockerConfigJSON, ok := secret.Data[".dockerconfigjson"]
	if !ok {
		return nil, fmt.Errorf("secret is missing .dockerconfigjson key")
	}

	var parsed struct {
		Auths map[string]struct {
			Auth  string `json:"auth"`
			Email string `json:"email"`
		} `json:"auths"`
	}
	if err := json.Unmarshal(dockerConfigJSON, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse pull secret JSON: %w", err)
	}

	entry, found := parsed.Auths[authID]
	if !found {
		return nil, fmt.Errorf("auth '%s' not found in pull secret", authID)
	}

	return &pullSecretAuthEntry{
		auth:  entry.Auth,
		email: entry.Email,
	}, nil
}

const (
	checkSyncMaxAttempts = 24 // 24 × 5s = 2 minute total timeout for SyncSet sync
	syncPollInterval     = 5 * time.Second
)

// MergePullSecretAuths merges new auths into existing pull secret data. Existing auths
// not present in newData are preserved. This never removes auths.
func MergePullSecretAuths(existingData, newData []byte) ([]byte, error) {
	type auth struct {
		Auth  string `json:"auth"`
		Email string `json:"email"`
	}
	type auths struct {
		Auths map[string]auth `json:"auths"`
	}

	var existing, incoming auths

	if err := json.Unmarshal(existingData, &existing); err != nil {
		return nil, fmt.Errorf("failed to parse existing pull secret: %w", err)
	}
	if err := json.Unmarshal(newData, &incoming); err != nil {
		return nil, fmt.Errorf("failed to parse new pull secret: %w", err)
	}

	if existing.Auths == nil {
		existing.Auths = make(map[string]auth)
	}

	for k, v := range incoming.Auths {
		if v.Auth == "" {
			continue
		}
		existing.Auths[k] = v
	}

	return json.Marshal(existing)
}

// UpdateHivePullSecretSSS updates the pull secret in the given hive namespace
// using update-in-place (never deletes). If the secret doesn't exist, it creates it.
// When the secret exists, new auths are merged into the existing secret via MergePullSecretAuths,
// preserving any auths not present in the new data.
func UpdateHivePullSecretSSS(ctx context.Context, kubeCli client.Client, clientset *kubernetes.Clientset, hiveNamespace string, cdName string, pullsecret []byte, out io.Writer) error {
	// Check for conflicting SyncSets before any mutations.
	// If the user aborts here, no secrets have been modified.
	if err := CheckExistingSyncSets(ctx, hiveNamespace, kubeCli, out); err != nil {
		return err
	}

	secretName := "pull"

	existing, err := clientset.CoreV1().Secrets(hiveNamespace).Get(ctx, secretName, metav1.GetOptions{})
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to read secret %s/%s: %w", hiveNamespace, secretName, err)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: hiveNamespace,
			},
			Type: corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				".dockerconfigjson": pullsecret,
			},
		}
		_, createErr := clientset.CoreV1().Secrets(hiveNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if createErr != nil {
			return fmt.Errorf("failed to create secret %s/%s: %w", hiveNamespace, secretName, createErr)
		}
	} else {
		existingData, ok := existing.Data[".dockerconfigjson"]
		if !ok || len(existingData) == 0 {
			fmt.Fprintf(out, "%s secret %s/%s exists but missing .dockerconfigjson key — overwriting\n", psColorWarn("[WARN]"), hiveNamespace, secretName)
			existing.Data[".dockerconfigjson"] = pullsecret
		} else {
			mergedData, mergeErr := MergePullSecretAuths(existingData, pullsecret)
			if mergeErr != nil {
				return fmt.Errorf("failed to merge pull secret auths for %s/%s: %w", hiveNamespace, secretName, mergeErr)
			}
			existing.Data[".dockerconfigjson"] = mergedData
		}
		_, updateErr := clientset.CoreV1().Secrets(hiveNamespace).Update(ctx, existing, metav1.UpdateOptions{})
		if updateErr != nil {
			return fmt.Errorf("failed to update secret %s/%s: %w", hiveNamespace, secretName, updateErr)
		}
	}

	fmt.Fprintf(out, "[OK] hive secret %s/pull updated\n", hiveNamespace)

	if err := syncPullSecretViaHive(ctx, hiveNamespace, cdName, kubeCli, out); err != nil {
		return fmt.Errorf("hive secret updated but sync to target failed: %w. Re-run this command to retry the sync", err)
	}

	return nil
}

// SyncSetName is the name used by this tool for pull secret SyncSets.
// Distinct from "pull-secret-replacement" used by transfer-owner to avoid collisions.
const SyncSetName = "pull-secret-update"

// CheckExistingSyncSets checks for existing SyncSets that could interfere with
// a new pull secret sync. Must be called BEFORE updating the hive secret so that
// aborting leaves no mutations.
func CheckExistingSyncSets(ctx context.Context, hiveNamespace string, kubeCli client.Client, out io.Writer) error {
	for _, ssName := range []string{SyncSetName, "pull-secret-replacement"} {
		existing := &hiveapiv1.SyncSet{}
		existing.Name = ssName
		existing.Namespace = hiveNamespace
		if err := kubeCli.Get(ctx, client.ObjectKeyFromObject(existing), existing); err == nil {
			fmt.Fprintf(out, "\n%s Existing SyncSet %s/%s found.\n", psColorWarn("[WARN]"), hiveNamespace, ssName)
			if ssName == "pull-secret-replacement" {
				fmt.Fprintf(out, "  This SyncSet was created by transfer-owner or a previous tool.\n")
			} else {
				fmt.Fprintf(out, "  This SyncSet was created by a previous pull-secret update run.\n")
			}
			age := time.Since(existing.CreationTimestamp.Time)
			fmt.Fprintf(out, "  Created: %s (%s ago)\n", existing.CreationTimestamp.Format("2006-01-02 15:04:05 UTC"), age.Truncate(time.Second))
			if age < 5*time.Minute {
				fmt.Fprintf(out, "  %s Created recently — another SRE may be running this tool on the same cluster.\n", psColorWarn("[WARN]"))
			}
			fmt.Fprintf(out, "\n  No changes have been made yet — it is safe to abort.\n")
			fmt.Fprintf(out, "\n  Options:\n")
			fmt.Fprintf(out, "    1. Delete it and continue (recommended if orphaned)\n")
			fmt.Fprintf(out, "    2. Abort — investigate manually (recommended if concurrent)\n")
			fmt.Fprintf(out, "  Delete existing SyncSet and continue? ")

			reader := bufio.NewReader(os.Stdin)
			response, readErr := reader.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				return fmt.Errorf("failed to read user input: %w", readErr)
			}
			response = strings.TrimSpace(strings.ToLower(response))
			if response != "y" && response != "yes" {
				return fmt.Errorf("aborted — existing SyncSet %s/%s needs manual investigation", hiveNamespace, ssName)
			}

			fmt.Fprintf(out, "  Deleting SyncSet %s/%s...\n", hiveNamespace, ssName)
			if delErr := kubeCli.Delete(ctx, existing); delErr != nil {
				return fmt.Errorf("failed to delete existing SyncSet %s/%s: %w", hiveNamespace, ssName, delErr)
			}
			time.Sleep(5 * time.Second)
		}
	}
	return nil
}

// syncPullSecretViaHive creates a SyncSet to sync the pull secret from hive to
// the target cluster, polls ClusterSync for completion, then cleans up the SyncSet.
// CheckExistingSyncSets must be called before this function.
func syncPullSecretViaHive(ctx context.Context, hiveNamespace string, cdName string, kubeCli client.Client, out io.Writer) error {
	syncSet := &hiveapiv1.SyncSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      SyncSetName,
			Namespace: hiveNamespace,
		},
		Spec: hiveapiv1.SyncSetSpec{
			ClusterDeploymentRefs: []corev1.LocalObjectReference{
				{Name: cdName},
			},
			SyncSetCommonSpec: hiveapiv1.SyncSetCommonSpec{
				ResourceApplyMode: "Upsert",
				Secrets: []hiveapiv1.SecretMapping{
					{
						SourceRef: hiveapiv1.SecretReference{
							Name:      "pull",
							Namespace: hiveNamespace,
						},
						TargetRef: hiveapiv1.SecretReference{
							Name:      "pull-secret",
							Namespace: "openshift-config",
						},
					},
				},
			},
		},
	}

	if err := kubeCli.Create(ctx, syncSet); err != nil {
		return fmt.Errorf("failed to create SyncSet: %w", err)
	}
	// Use the server-side creation timestamp for comparison, not local time.
	// Kubernetes timestamps have second-level precision; comparing against
	// a nanosecond-precision local time.Now() causes false negatives when
	// the sync completes within the same second as the creation.
	syncSetCreatedAt := syncSet.CreationTimestamp.Time
	fmt.Fprintf(out, "SyncSet %s in namespace %s has been created.\n", SyncSetName, hiveNamespace)

	if err := hiveinternalv1alpha1.AddToScheme(kubeCli.Scheme()); err != nil {
		return fmt.Errorf("failed to add hiveinternal scheme: %w", err)
	}

	searchStatus := &hiveinternalv1alpha1.ClusterSync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cdName,
			Namespace: hiveNamespace,
		},
	}
	foundStatus := &hiveinternalv1alpha1.ClusterSync{}
	isSynced := false
	var lastGetErr error
	reader := bufio.NewReader(os.Stdin)

	// Poll in 60s rounds (12 × 5s), prompt user to continue or abort each round
	for round := 0; ; round++ {
		for i := 0; i < 12; i++ {
			if err := kubeCli.Get(ctx, client.ObjectKeyFromObject(searchStatus), foundStatus); err != nil {
				lastGetErr = err
				fmt.Fprintf(out, "!")
				time.Sleep(syncPollInterval)
				continue
			}
			lastGetErr = nil

			for _, status := range foundStatus.Status.SyncSets {
				if status.Name == SyncSetName && status.FirstSuccessTime != nil {
					if !status.FirstSuccessTime.Time.Before(syncSetCreatedAt) {
						isSynced = true
						break
					}
				}
			}

			if isSynced {
				fmt.Fprintf(out, "\nSync completed...\n")
				break
			}

			fmt.Fprintf(out, ".")
			time.Sleep(syncPollInterval)
		}

		if isSynced {
			break
		}

		fmt.Fprintf(out, "\n%s SyncSet sync not confirmed after %d seconds.\n", psColorWarn("[WARN]"), (round+1)*60)
		if lastGetErr != nil {
			fmt.Fprintf(out, "  Last error: %v\n", lastGetErr)
		}
		fmt.Fprintf(out, "Continue waiting? (y/N): ")
		response, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			break
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			break
		}
	}

	// Always clean up the SyncSet, even on timeout
	if delErr := kubeCli.Delete(ctx, syncSet); delErr != nil {
		fmt.Fprintf(out, "\n%s failed to delete SyncSet %s/%s: %v\n", psColorWarn("[WARN]"), hiveNamespace, SyncSetName, delErr)
	}

	if !isSynced {
		if lastGetErr != nil {
			return fmt.Errorf("SyncSet %s/%s sync not confirmed (last error: %v) (SyncSet cleaned up)", hiveNamespace, SyncSetName, lastGetErr)
		}
		return fmt.Errorf("SyncSet %s/%s sync not confirmed (SyncSet cleaned up). Re-run this command to retry", hiveNamespace, SyncSetName)
	}

	return nil
}

// UpdateHCPPullSecretViaManifestWork updates the pull secret within a ManifestWork
// on the service cluster for HCP clusters.
//
// HCP pull secret architecture:
//   - This operates at level 1 (HostedCluster.spec.pullSecret)
//   - HCCO reconciles changes to kube-system/original-pull-secret on the hosted cluster
//   - Customer-added registries in kube-system/additional-pull-secret are not affected
//   - Ref: https://access.redhat.com/solutions/7118834
//   - Ref: https://hypershift.pages.dev/how-to/powervs/global-pull-secret/
func UpdateHCPPullSecretViaManifestWork(ctx context.Context, ocm *sdk.Connection, kubeCli client.Client, clusterID, mgmtClusterName string, pullsecret []byte, out io.Writer) error {
	if err := workv1.AddToScheme(kubeCli.Scheme()); err != nil {
		return fmt.Errorf("failed to add work scheme: %w", err)
	}

	hostedCluster, err := utils.GetClusterAnyStatus(ocm, clusterID)
	if err != nil {
		return fmt.Errorf("failed to get cluster: %w", err)
	}

	secretNamePrefix := hostedCluster.DomainPrefix() + "-pull"
	newSecretName := secretNamePrefix + "-" + randomHexSuffix(6)

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		manifestWork := &workv1.ManifestWork{}
		if err := kubeCli.Get(timeoutCtx, types.NamespacedName{Name: clusterID, Namespace: mgmtClusterName}, manifestWork); err != nil {
			return fmt.Errorf("failed to get ManifestWork %s/%s: %w", mgmtClusterName, clusterID, err)
		}

		if err := updateManifestWorkPayloads(manifestWork, secretNamePrefix, newSecretName, pullsecret); err != nil {
			return err
		}

		return kubeCli.Update(timeoutCtx, manifestWork, &client.UpdateOptions{})
	})
	if err != nil {
		return fmt.Errorf("cannot update pull-secret within ManifestWork: %w", err)
	}

	fmt.Fprintf(out, "ManifestWork updated. Waiting for secret to sync on hosted cluster...\n")

	// Poll the ManifestWork status for applied condition rather than sleeping a fixed duration.
	// Uses the parent ctx (not timeoutCtx) since the user controls loop duration via prompts.
	// Each round polls for 60s (12 × 5s), then prompts to continue or abort.
	reader := bufio.NewReader(os.Stdin)
	for round := 0; ; round++ {
		for i := 0; i < 12; i++ {
			mw := &workv1.ManifestWork{}
			if getErr := kubeCli.Get(ctx, types.NamespacedName{Name: clusterID, Namespace: mgmtClusterName}, mw); getErr == nil {
				for _, cond := range mw.Status.Conditions {
					if cond.Type == "Applied" && cond.Status == "True" {
						fmt.Fprintf(out, "\nManifestWork applied.\n")
						return nil
					}
				}
			}
			fmt.Fprintf(out, ".")
			time.Sleep(5 * time.Second)
		}
		fmt.Fprintf(out, "\n%s ManifestWork sync not confirmed after %d seconds.\n", psColorWarn("[WARN]"), (round+1)*60)
		fmt.Fprintf(out, "Continue waiting? (y/N): ")
		response, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("failed to read user input: %w", readErr)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Fprintf(out, "%s Aborting wait — verify ManifestWork sync manually.\n", psColorWarn("[WARN]"))
			return nil
		}
	}
}

func updateManifestWorkPayloads(mw *workv1.ManifestWork, secretNamePrefix, newSecretName string, pullsecret []byte) error {
	secretUpdated := false
	hcIndex := -1

	for i, manifest := range mw.Spec.Workload.Manifests {
		if manifest.Raw == nil {
			continue
		}

		var meta struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(manifest.Raw, &meta); err != nil {
			return err
		}

		switch meta.Kind {
		case "Secret":
			secret := &corev1.Secret{}
			if err := json.Unmarshal(manifest.Raw, secret); err != nil {
				return err
			}
			if strings.HasPrefix(secret.Name, secretNamePrefix) {
				if secret.Data == nil {
					secret.Data = map[string][]byte{}
				}
				oldPullSecret, hasKey := secret.Data[".dockerconfigjson"]
				var newPullSecret []byte
				if !hasKey || len(oldPullSecret) == 0 {
					newPullSecret = pullsecret
				} else {
					var mergeErr error
					newPullSecret, mergeErr = MergePullSecretAuths(oldPullSecret, pullsecret)
					if mergeErr != nil {
						return fmt.Errorf("cannot merge pull secret auths: %w", mergeErr)
					}
				}
				secret.Name = newSecretName
				secret.Data[".dockerconfigjson"] = newPullSecret
				secretJSON, err := json.Marshal(secret)
				if err != nil {
					return err
				}
				mw.Spec.Workload.Manifests[i].Raw = secretJSON
				secretUpdated = true
			}
		case "HostedCluster":
			hcIndex = i
		}
	}

	if !secretUpdated {
		return fmt.Errorf("no Secret matching prefix %q found in ManifestWork", secretNamePrefix)
	}
	if hcIndex >= 0 {
		hc := &hypershiftv1beta1.HostedCluster{}
		if err := json.Unmarshal(mw.Spec.Workload.Manifests[hcIndex].Raw, hc); err != nil {
			return err
		}
		hc.Spec.PullSecret.Name = newSecretName
		hcJSON, err := json.Marshal(hc)
		if err != nil {
			return err
		}
		mw.Spec.Workload.Manifests[hcIndex].Raw = hcJSON
	}

	return nil
}

// RestartPodsBySelector deletes pods matching the selector in the namespace to trigger a rollout.
func RestartPodsBySelector(ctx context.Context, clientset *kubernetes.Clientset, namespace, selector string, out io.Writer) error {
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return fmt.Errorf("failed to list pods in namespace '%s' with selector '%s': %w", namespace, selector, err)
	}

	for _, pod := range pods.Items {
		if err := clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{}); err != nil {
			return fmt.Errorf("failed to delete pod '%s' in namespace '%s': %w", pod.Name, namespace, err)
		}
		fmt.Fprintf(out, "Pod %s in namespace %s has been deleted.\n", pod.Name, namespace)
	}

	fmt.Fprintf(out, "Pods in namespace %s with selector '%s' have been deleted.\n", namespace, selector)
	return nil
}

func randomHexSuffix(length int) string {
	const chars = "0123456789abcdef"
	result := make([]byte, length)
	for i := range result {
		result[i] = chars[rand.Intn(len(chars))] //nolint:gosec // resource name suffix, not security-sensitive
	}
	return string(result)
}
