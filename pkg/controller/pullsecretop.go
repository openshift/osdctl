package controller

import (
	"context"
	"fmt"
	"io"

	"github.com/fatih/color"
	sdk "github.com/openshift-online/ocm-sdk-go"
	amv1 "github.com/openshift-online/ocm-sdk-go/accountsmgmt/v1"
	"github.com/sirupsen/logrus"
	authorizationv1 "k8s.io/api/authorization/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	opColorOK     = color.New(color.FgGreen).SprintFunc()
	opColorFail   = color.New(color.FgRed).SprintFunc()
	opColorWarn   = color.New(color.FgYellow).SprintFunc()
	opColorDryRun = color.New(color.FgCyan).SprintFunc()
	opColorHdr    = color.New(color.FgBlue, color.Bold).SprintFunc()
	opColorDetail = color.New(color.FgWhite).SprintFunc()
)

// PullSecretOp carries context for pull secret operations. Each method
// checks DryRun and either performs the operation or reports what it would do.
type PullSecretOp struct {
	DryRun             bool
	Logger             *logrus.Logger
	Out                io.Writer
	AllOK              bool
	PullSecretUpToDate bool
	PullSecretUpdated  bool
	AuthDiffCount      int
	Failures           []string
}

// NewPullSecretOp creates a new operation context.
func NewPullSecretOp(dryRun bool, logger *logrus.Logger, out io.Writer) *PullSecretOp {
	return &PullSecretOp{
		DryRun: dryRun,
		Logger: logger,
		Out:    out,
		AllOK:  true,
	}
}

// Section prints a step header with educational description.
func (op *PullSecretOp) Section(step int, title string, lines ...string) {
	fmt.Fprintf(op.Out, "\n%s\n", opColorHdr("============================================================"))
	prefix := ""
	if op.DryRun {
		prefix = opColorDryRun("[Dry Run] ")
	}
	fmt.Fprintf(op.Out, "%s%s\n", prefix, opColorHdr(fmt.Sprintf("Step %d: %s", step, title)))
	for _, line := range lines {
		fmt.Fprintf(op.Out, "  %s\n", opColorDetail(line))
	}
	fmt.Fprintf(op.Out, "%s\n", opColorHdr("============================================================"))
}

// OK prints a success result.
func (op *PullSecretOp) OK(format string, args ...any) {
	prefix := ""
	if op.DryRun {
		prefix = opColorDryRun("[Dry Run] ")
	}
	fmt.Fprintf(op.Out, "%s%s %s\n", prefix, opColorOK("[OK]"), fmt.Sprintf(format, args...))
}

// Fail prints a failure result and marks the operation as not-all-OK.
func (op *PullSecretOp) Fail(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	prefix := ""
	if op.DryRun {
		prefix = opColorDryRun("[Dry Run] ")
	}
	fmt.Fprintf(op.Out, "%s%s %s\n", prefix, opColorFail("[FAIL]"), msg)
	op.AllOK = false
	op.Failures = append(op.Failures, msg)
}

// Warn prints a warning.
func (op *PullSecretOp) Warn(format string, args ...any) {
	prefix := ""
	if op.DryRun {
		prefix = opColorDryRun("[Dry Run] ")
	}
	fmt.Fprintf(op.Out, "%s%s %s\n", prefix, opColorWarn("[WARN]"), fmt.Sprintf(format, args...))
}

// Would prints what the operation would do (dry-run only).
func (op *PullSecretOp) Would(format string, args ...any) {
	if op.DryRun {
		fmt.Fprintf(op.Out, "%s %s %s\n", opColorDryRun("[Dry Run]"), opColorDryRun("Would:"), opColorDryRun(fmt.Sprintf(format, args...)))
	}
}

// Info prints an informational message.
func (op *PullSecretOp) Info(format string, args ...any) {
	prefix := ""
	if op.DryRun {
		prefix = opColorDryRun("[Dry Run] ")
	}
	fmt.Fprintf(op.Out, "%s%s\n", prefix, fmt.Sprintf(format, args...))
}

// CheckCanI verifies RBAC permission. In dry-run mode it reports the result.
// In live mode it just logs the check. Returns whether the permission is allowed.
func (op *PullSecretOp) CheckCanI(ctx context.Context, clientset *kubernetes.Clientset, systemLabel, verb, resource, group, namespace string) bool {
	if clientset == nil {
		op.AllOK = false
		if op.DryRun {
			fmt.Fprintf(op.Out, "%s %s %s: auth can-i %s %s in %s (client not available)\n",
				opColorDryRun("[Dry Run]"), opColorWarn("[SKIP]"), systemLabel, verb, resource, nsLabel(namespace))
		}
		return false
	}

	review := &authorizationv1.SelfSubjectAccessReview{
		Spec: authorizationv1.SelfSubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Verb:      verb,
				Resource:  resource,
				Group:     group,
				Namespace: namespace,
			},
		},
	}
	result, err := clientset.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		op.AllOK = false
		if op.DryRun {
			fmt.Fprintf(op.Out, "%s %s %s: auth can-i %s %s in %s (%v)\n",
				opColorDryRun("[Dry Run]"), opColorWarn("[SKIP]"), systemLabel, verb, resource, nsLabel(namespace), err)
		}
		return false
	}

	allowed := result.Status.Allowed
	if !allowed {
		op.AllOK = false
	}
	if op.DryRun {
		status := opColorOK("[OK]")
		if !allowed {
			status = opColorFail("[FAIL]")
		}
		fmt.Fprintf(op.Out, "%s %s %s: auth can-i %s %s in %s\n",
			opColorDryRun("[Dry Run]"), status, systemLabel, verb, resource, nsLabel(namespace))
	} else if !allowed {
		op.Warn("%s: insufficient permission — cannot %s %s in %s", systemLabel, verb, resource, nsLabel(namespace))
	}
	return allowed
}

// CheckSecretExists checks if a secret exists. Returns true if found.
func (op *PullSecretOp) CheckSecretExists(ctx context.Context, clientset *kubernetes.Clientset, namespace, name, systemLabel string) bool {
	if clientset == nil {
		op.Fail("cannot check secret %s/%s — client not available", namespace, name)
		return false
	}

	_, err := clientset.CoreV1().Secrets(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			op.Warn("secret %s/%s not found on %s", namespace, name, systemLabel)
		} else {
			op.Fail("failed to read secret %s/%s on %s: %v", namespace, name, systemLabel, err)
		}
		return false
	}
	op.OK("secret %s/%s exists on %s", namespace, name, systemLabel)
	return true
}

// FindHiveNamespaceOp wraps FindHiveNamespace with operational output.
func (op *PullSecretOp) FindHiveNamespaceOp(ctx context.Context, kubeCli client.Client, clusterID, infraName string) (*HiveNamespaceInfo, bool) {
	op.Info("Resolving Hive namespace for cluster %s on %s...", clusterID, infraName)
	hiveInfo, err := FindHiveNamespace(ctx, kubeCli, clusterID)
	if err != nil {
		op.Fail("could not find Hive namespace: %v", err)
		return nil, false
	}
	op.OK("found ClusterDeployment %s/%s on %s", hiveInfo.Namespace, hiveInfo.ClusterDeploymentName, infraName)
	return hiveInfo, true
}

// FetchAccessTokenOp wraps FetchOwnerAccessToken with operational output.
func (op *PullSecretOp) FetchAccessTokenOp(ocm *sdk.Connection, ownerUsername string) ([]byte, map[string]*amv1.AccessTokenAuth, bool) {
	op.Logger.Infof("Fetching pull secret from OCM for owner '%s'", ownerUsername)
	pullSecret, auths, err := FetchOwnerAccessToken(ocm, ownerUsername, op.Logger)
	if err != nil {
		op.Fail("could not fetch OCM access token: %v", err)
		return nil, nil, false
	}
	op.OK("retrieved %d auth entries from OCM access token", len(auths))

	missing := ValidateRequiredAuths(auths)
	if len(missing) > 0 {
		for _, m := range missing {
			op.Warn("OCM access token missing required auth: %s", m)
		}
	}
	return pullSecret, auths, true
}

// ResolveExistingPullSecret finds the best available base pull secret data.
// Tries the hive secret first, then falls back to the target cluster's secret.
// Returns the secret data bytes and the source description.
func (op *PullSecretOp) ResolveExistingPullSecret(ctx context.Context, infraClientSet *kubernetes.Clientset, targetClientSet *kubernetes.Clientset, hiveNS string, infraName string, targetName string) ([]byte, string) {
	// Try hive secret first
	if infraClientSet != nil {
		hiveSecret, err := infraClientSet.CoreV1().Secrets(hiveNS).Get(ctx, "pull", metav1.GetOptions{})
		if err == nil {
			if data, ok := hiveSecret.Data[".dockerconfigjson"]; ok {
				op.OK("secret %s/pull found on %s — using as base", hiveNS, infraName)
				return data, fmt.Sprintf("%s/pull on %s", hiveNS, infraName)
			}
		}
		op.Warn("secret %s/pull not found on %s", hiveNS, infraName)
	}

	// Hive secret missing — check target cluster
	if targetClientSet != nil {
		targetSecret, err := targetClientSet.CoreV1().Secrets("openshift-config").Get(ctx, "pull-secret", metav1.GetOptions{})
		if err == nil {
			if data, ok := targetSecret.Data[".dockerconfigjson"]; ok {
				op.OK("secret openshift-config/pull-secret found on %s (can be used as base)", targetName)
				return data, fmt.Sprintf("openshift-config/pull-secret on %s", targetName)
			}
		}
		op.Warn("secret openshift-config/pull-secret not found on %s", targetName)
	}

	// Neither found
	op.Warn("no existing pull secret found — will need to build from OCM auths only")
	return nil, ""
}

func nsLabel(namespace string) string {
	if namespace == "" {
		return "(cluster-scoped)"
	}
	return namespace
}
