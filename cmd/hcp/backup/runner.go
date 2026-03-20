package backup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift/osdctl/cmd/cluster"
	logrus "github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// backupIDPattern matches the backup name from velero's submission output, e.g.:
//
//	Backup request "2p49ok746l9e7o06v76t3ptp95k72heb-daily-20260319184212" submitted successfully.
var backupIDPattern = regexp.MustCompile(`Backup request "([^"]+)" submitted`)

// joinSortedMap returns a comma-separated "key=value,..." string built from m
// with keys in ascending alphabetical order, suitable for Velero's --labels /
// --annotations flags.
func joinSortedMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+m[k])
	}
	return strings.Join(pairs, ",")
}

// KubeClient is a generic interface for interacting with Kubernetes resources.
// It mirrors controller-runtime's Reader for Get and List, and adds Exec for
// in-pod command execution. Both privileged and unprivileged clients satisfy
// this interface, making them interchangeable in the runner and in tests.
type KubeClient interface {
	// Get retrieves the resource identified by key and populates obj with the result.
	// obj must be a pointer to an initialized struct (e.g. &corev1.Pod{}).
	Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error

	// List retrieves a collection of resources and populates list with the result.
	// Namespace and label filters are passed as ListOptions (e.g. client.InNamespace, client.MatchingLabels).
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error

	// Exec runs cmd inside the named container of the given pod and returns its stdout.
	// It is only supported by clients constructed with a REST config and Clientset.
	Exec(ctx context.Context, namespace, pod, container string, cmd []string) (string, error)
}

// kubeClient is the production implementation of KubeClient. runtimeCli handles
// Get and List via the controller-runtime API. restCfg and clientset are used
// exclusively by Exec to build a SPDY executor.
type kubeClient struct {
	runtimeCli client.Client
	restCfg    *rest.Config
	clientset  kubernetes.Interface
}

// newKubeClient constructs a kubeClient. restCfg and clientset may be nil for
// read-only clients that will never call Exec.
func newKubeClient(runtimeCli client.Client, restCfg *rest.Config, clientset kubernetes.Interface) *kubeClient {
	return &kubeClient{
		runtimeCli: runtimeCli,
		restCfg:    restCfg,
		clientset:  clientset,
	}
}

func (k *kubeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return k.runtimeCli.Get(ctx, key, obj, opts...)
}

func (k *kubeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	return k.runtimeCli.List(ctx, list, opts...)
}

// Exec runs cmd inside the named container of the given pod using a SPDY executor.
// It returns an error if the client was constructed without a REST config or Clientset.
func (k *kubeClient) Exec(ctx context.Context, namespace, pod, container string, cmd []string) (string, error) {
	if k.restCfg == nil || k.clientset == nil {
		return "", errors.New("Exec is not supported on this client: no REST config or Clientset provided")
	}

	req := k.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(pod).
		Namespace(namespace).
		SubResource("exec")

	option := &corev1.PodExecOptions{
		Container: container,
		Command:   cmd,
		Stdin:     false,
		Stdout:    true,
		Stderr:    true,
		TTY:       false,
	}

	req.VersionedParams(option, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(k.restCfg, "POST", req.URL())
	if err != nil {
		return "", fmt.Errorf("creating SPDY executor for pod exec: %w", err)
	}

	stdoutCapture := &cluster.LogCapture{}
	stderrCapture := &cluster.LogCapture{}

	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: stdoutCapture,
		Stderr: stderrCapture,
		Tty:    false,
	})
	if err != nil {
		return "", fmt.Errorf("executing command in pod %q: %w\nstderr: %s", pod, err, stderrCapture.GetStdOut())
	}

	return stdoutCapture.GetStdOut(), nil
}

// Printer writes user-facing output (command results, hints) to an output stream.
// Diagnostic/progress logging uses *logrus.Logger instead.
type Printer interface {
	Print(s string)
	Printf(format string, args ...any)
}

// defaultPrinter writes to an io.Writer (typically cmd.OutOrStdout()).
type defaultPrinter struct {
	w io.Writer
}

func (p *defaultPrinter) Print(s string) {
	fmt.Fprint(p.w, s)
}

func (p *defaultPrinter) Printf(format string, args ...any) {
	fmt.Fprintf(p.w, format, args...)
}

// defaultBackupRunnerConfig holds all configurable parameters for the backup runner,
// including operational settings and injectable dependencies (logger, printer,
// resolver, builderFactory). Use newDefaultBackupRunnerConfig to obtain an
// instance with defaults applied.
type defaultBackupRunnerConfig struct {
	ADPNamespace       string
	VeleroContainer    string
	VeleroLabelKey     string
	VeleroLabelValue   string
	ScheduleNameSuffix string
	Logger             *logrus.Logger
	Printer            Printer
	// Resolver resolves a raw cluster identifier to canonical OCM IDs.
	// Defaults to an ocmClusterResolver constructed from the OCM connection
	// passed to NewDefaultBackupRunner. Override via WithResolver in tests.
	Resolver ClusterResolver
	// Builder creates KubeClient instances. Defaults to a backplaneClientBuilder
	// constructed from the OCM connection passed to NewDefaultBackupRunner.
	// Override via WithBuilder in tests.
	Builder KubeClientBuilder
}

// newDefaultBackupRunnerConfig returns a defaultBackupRunnerConfig populated with
// standard defaults. Call it once, then pass any overrides via DefaultBackupRunnerOption.
// Resolver and Builder are left nil here; NewDefaultBackupRunner fills them in
// from the OCM connection if they have not been overridden by opts.
func newDefaultBackupRunnerConfig(opts ...DefaultBackupRunnerOption) defaultBackupRunnerConfig {
	cfg := defaultBackupRunnerConfig{
		ADPNamespace:       "openshift-adp",
		VeleroContainer:    "velero",
		VeleroLabelKey:     "app.kubernetes.io/name",
		VeleroLabelValue:   "velero",
		ScheduleNameSuffix: "-daily",
		Logger:             logrus.New(),
		Printer:            &defaultPrinter{w: os.Stdout},
	}
	for _, o := range opts {
		o.ConfigureDefaultBackupRunner(&cfg)
	}
	return cfg
}

// DefaultBackupRunnerOption is implemented by any type that can configure a defaultBackupRunnerConfig.
// Concrete implementations live in options.go.
type DefaultBackupRunnerOption interface {
	ConfigureDefaultBackupRunner(*defaultBackupRunnerConfig)
}

// defaultBackupRunner executes the backup workflow for an HCP cluster.
// All network I/O — OCM cluster resolution, backplane login, and Kubernetes
// operations — is deferred to Run so that construction is always cheap and
// side-effect free.
//
// logger and printer are promoted from cfg at construction time so that methods
// can reference them directly (r.logger, r.printer) rather than through
// r.cfg.Logger / r.cfg.Printer on every call.
type defaultBackupRunner struct {
	cfg      defaultBackupRunnerConfig
	logger   *logrus.Logger
	printer  Printer
	resolver ClusterResolver
	builder  KubeClientBuilder
}

// NewDefaultBackupRunner constructs a defaultBackupRunner that will use ocmConn
// for all OCM API calls made during Run. opts are applied after defaults and may
// override any field, including Resolver and BuilderFactory (useful in tests to
// avoid real network calls).
func NewDefaultBackupRunner(
	ocmConn *ocmsdk.Connection,
	opts ...DefaultBackupRunnerOption,
) *defaultBackupRunner {
	cfg := newDefaultBackupRunnerConfig(opts...)

	resolver := cfg.Resolver
	if resolver == nil {
		resolver = &ocmClusterResolver{ocmConn: ocmConn, logger: cfg.Logger}
	}

	builder := cfg.Builder
	if builder == nil {
		builder = &backplaneClientBuilder{ocmConn: ocmConn, logger: cfg.Logger}
	}

	return &defaultBackupRunner{
		cfg:      cfg,
		logger:   cfg.Logger,
		printer:  cfg.Printer,
		resolver: resolver,
		builder:  builder,
	}
}

// Run executes the backup workflow: resolve cluster, validate schedule, find
// pod, trigger backup. All network I/O happens here — nothing is pre-fetched
// at construction time.
func (r *defaultBackupRunner) Run(ctx context.Context, flags *backupFlags) error {
	// Resolve the HCP cluster and its management cluster using the shared OCM
	// connection. This is the first network call in the workflow.
	clusterInfo, err := r.resolver.Resolve(ctx, flags.clusterID)
	if err != nil {
		return err
	}

	scheduleName := clusterInfo.HCPClusterID + r.cfg.ScheduleNameSuffix

	// Build an unprivileged client for read-only schedule validation.
	// The elevated login is intentionally deferred until after this check.
	readClient, err := r.builder.Build(ctx, WithClusterID{ClusterID: clusterInfo.MgmtClusterID})
	if err != nil {
		return err
	}

	// Validate the Velero Schedule resource exists before attempting the backup.
	// This is done with the unprivileged client so that a missing schedule never
	// triggers an elevated login.
	r.logger.Infof("Validating Velero schedule %q in namespace %q...", scheduleName, r.cfg.ADPNamespace)
	if err := r.validateSchedule(ctx, readClient, scheduleName); err != nil {
		return err
	}
	r.logger.Infof("Schedule %q found.", scheduleName)

	// Schedule validation passed — now build the elevated client for pod
	// listing and exec.
	execClient, err := r.builder.Build(ctx, WithClusterID{ClusterID: clusterInfo.MgmtClusterID}, WithElevation{Reason: flags.reason})
	if err != nil {
		return err
	}

	// Find a running Velero pod to exec into
	podName, err := r.findVeleroPod(ctx, execClient)
	if err != nil {
		return err
	}
	r.logger.Infof("Found Velero pod: %s", podName)

	// Trigger the backup
	backupCmd := []string{"./velero", "backup", "create", "--from-schedule", scheduleName}

	// Append user-supplied labels as a single --labels key=value,... argument.
	// Keys are sorted for determinism.
	if len(flags.labels) > 0 {
		backupCmd = append(backupCmd, "--labels", joinSortedMap(flags.labels))
	}

	// Append user-supplied annotations as a single --annotations key=value,... argument.
	// Keys are sorted for determinism.
	if len(flags.annotations) > 0 {
		backupCmd = append(backupCmd, "--annotations", joinSortedMap(flags.annotations))
	}

	r.logger.Infof("Triggering backup from schedule %q...", scheduleName)

	output, err := execClient.Exec(ctx, r.cfg.ADPNamespace, podName, r.cfg.VeleroContainer, backupCmd)
	if err != nil {
		return fmt.Errorf("triggering Velero backup from schedule %q: %w", scheduleName, err)
	}

	// Parse the backup ID from the velero submission output.
	// Example output:
	//   Creating backup from schedule, all other filters are ignored.
	//   Backup request "2p49ok746l9e7o06v76t3ptp95k72heb-daily-20260319184212" submitted successfully.
	//   Run `velero backup describe ...` or `velero backup logs ...` for more details.
	matches := backupIDPattern.FindStringSubmatch(output)
	if len(matches) < 2 {
		// Unexpected output format — print raw output so the operator can inspect it.
		r.printer.Print(output)
		r.printer.Printf("Backup triggered successfully, but could not parse backup ID from velero output.\n")
		return nil
	}
	backupID := matches[1]

	r.printer.Printf("Backup %q triggered successfully.\n", backupID)
	r.printer.Printf("To check status, run:\n")
	r.printer.Printf("oc get backup %s -n %s\n", backupID, r.cfg.ADPNamespace)

	return nil
}

// validateSchedule checks that a Velero Schedule CR with the given name exists
// in the ADP namespace. It uses an unstructured lookup to avoid importing the
// Velero SDK. readClient must be an unprivileged KubeClient with Get access.
func (r *defaultBackupRunner) validateSchedule(ctx context.Context, readClient KubeClient, scheduleName string) error {
	schedule := &unstructured.Unstructured{}
	schedule.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "velero.io",
		Version: "v1",
		Kind:    "Schedule",
	})

	key := client.ObjectKey{
		Namespace: r.cfg.ADPNamespace,
		Name:      scheduleName,
	}

	if err := readClient.Get(ctx, key, schedule); err != nil {
		return fmt.Errorf("getting Velero schedule %q in namespace %q (expected name: <cluster-id>%s): %w",
			scheduleName, r.cfg.ADPNamespace, r.cfg.ScheduleNameSuffix, err)
	}

	return nil
}

// findVeleroPod returns the name of a Running, Ready, non-terminating Velero
// pod. execClient must be a privileged KubeClient with List access to pods in
// the ADP namespace.
func (r *defaultBackupRunner) findVeleroPod(ctx context.Context, execClient KubeClient) (string, error) {
	podList := &corev1.PodList{}
	if err := execClient.List(ctx, podList,
		client.InNamespace(r.cfg.ADPNamespace),
		client.MatchingLabels{r.cfg.VeleroLabelKey: r.cfg.VeleroLabelValue},
	); err != nil {
		return "", fmt.Errorf("listing Velero pods in namespace %s: %w", r.cfg.ADPNamespace, err)
	}

	for _, pod := range podList.Items {
		if pod.DeletionTimestamp != nil {
			continue // terminating — skip
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		ready := false
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if !ready {
			continue
		}
		return pod.Name, nil
	}

	labelSelector := fmt.Sprintf("%s=%s", r.cfg.VeleroLabelKey, r.cfg.VeleroLabelValue)
	return "", fmt.Errorf("no running Velero pod found in namespace %s with label %s", r.cfg.ADPNamespace, labelSelector)
}
