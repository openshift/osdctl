package mustgather

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	ocmsdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/rhobs"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type mustGather struct {
	clusterId          string
	reason             string
	gatherTargets      string
	acmMustGatherImage string
	hiveOcmUrl         string
	maxLogLines        int
	outputDir          string

	// Seams defaulted to the real implementations in NewCmdMustGather and overridden in tests,
	// so the orchestration (per-target outcomes, summary, exit policy) is exercisable without a
	// live cluster, backplane elevation, an `oc` binary, or RHOBS access.
	runMustGather func(*rest.Config, *kubernetes.Clientset, []string) error
	gatherHCPLogs func(context.Context, string, string, string, []string, time.Duration, int) (int, error)
}

func NewCmdMustGather() *cobra.Command {
	mg := &mustGather{
		runMustGather: createMustGather,
		gatherHCPLogs: rhobs.GatherHCPLogs,
	}

	mustGatherCommand := &cobra.Command{
		Use:   "must-gather --cluster-id <cluster-identifier>",
		Short: "Create a must-gather for an HCP cluster (and optionally its management/service clusters)",
		Long: `Create a must-gather for a HyperShift / ROSA HCP cluster.

--cluster-id is the cluster you want data about; --gather selects which cluster(s) are
actually collected from. The supplied ID and the gather targets are distinct: the default
target 'hcp' requires an HCP cluster ID, while 'mc'/'sc'/'sc_acm' can also be run directly
against a management- or service-cluster ID.

Gather targets:
  hcp     HCP control-plane logs & events from RHOBS, plus an ACM hypershift dump (requires an HCP ID)
  mc      management-cluster infrastructure must-gather
  sc      service-cluster infrastructure must-gather
  sc_acm  service-cluster ACM must-gather

Supported workflows:
  HCP cluster ID:         --gather hcp (default) | --gather mc | --gather sc,sc_acm | any combination
  Management cluster ID:  --gather mc
  Service cluster ID:     --gather sc,sc_acm

The 'hcp' target reads control-plane logs from RHOBS and therefore requires RHOBS client
credentials (RHOBS_CLIENT_ID / RHOBS_CLIENT_SECRET as env vars, in ~/.config/osdctl, or via Vault).`,
		Example: `  # HCP control-plane logs/events + hypershift dump (default)
  osdctl hcp must-gather --cluster-id ${HCP_CLUSTER_ID} --reason ${REASON}

  # HCP cluster plus its management and service clusters
  osdctl hcp must-gather --cluster-id ${HCP_CLUSTER_ID} --gather hcp,mc,sc,sc_acm --reason ${REASON}

  # Directly against a management-cluster ID
  osdctl hcp must-gather --cluster-id ${MC_CLUSTER_ID} --gather mc --reason ${REASON}`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return mg.Run()
		},
	}

	// To be changed to registry.redhat.io/multicluster-engine/must-gather-rhel9:v5.1 when it exists
	// The registry.redhat.io image requires authentication - see https://access.redhat.com/articles/RegistryAuthentication
	defaultAcmImage := "quay.io/redhat-user-workloads/crt-redhat-acm-tenant/must-gather-acm-51:203e21767f29c24069a92565638befed9fc7cc41"
	mustGatherCommand.Flags().StringVarP(&mg.clusterId, "cluster-id", "C", "", "Internal ID of the cluster to gather data from")
	mustGatherCommand.Flags().StringVar(&mg.reason, "reason", "", "The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).")
	mustGatherCommand.Flags().StringVar(&mg.gatherTargets, "gather", "hcp", "Comma-separated list of gather targets (available: hcp, mc, sc, sc_acm).")
	mustGatherCommand.Flags().StringVar(&mg.acmMustGatherImage, "acm_image", defaultAcmImage, "Overrides the acm must-gather image being used for acm mc, sc as well as hcp must-gathers.")
	mustGatherCommand.Flags().StringVar(&mg.hiveOcmUrl, "hive-ocm-url", "production", `OCM environment for the RHOBS/hive lookups used by the hcp target - aliases: "production", "staging", "integration".`)
	mustGatherCommand.Flags().IntVar(&mg.maxLogLines, "max-log-lines", 100000, "Maximum number of most-recent log lines to collect per namespace from RHOBS for the hcp target; a positive cap is written newest-first (0 = unlimited, oldest-first).")
	mustGatherCommand.Flags().StringVar(&mg.outputDir, "output-dir", "", "Directory to write the must-gather into (defaults to the current working directory). Avoid RAM-backed paths like /tmp (tmpfs) for large gathers.")

	mustGatherCommand.MarkFlagRequired("cluster-id")
	mustGatherCommand.MarkFlagRequired("reason")

	return mustGatherCommand
}

// validGatherTargets is the set of accepted --gather values.
var validGatherTargets = map[string]bool{"hcp": true, "mc": true, "sc": true, "sc_acm": true}

// parseAndValidateTargets splits the --gather value and rejects unknown targets or combinations
// incompatible with the supplied cluster ID, so the command fails with an actionable error before
// creating output or elevating into any cluster.
func parseAndValidateTargets(raw string, cluster *cmv1.Cluster) ([]string, error) {
	var targets []string
	seen := map[string]bool{}
	for _, target := range strings.Split(raw, ",") {
		target = strings.TrimSpace(target)
		if target == "" {
			continue
		}
		if !validGatherTargets[target] {
			return nil, fmt.Errorf("unknown gather target %q; valid targets are: hcp, mc, sc, sc_acm", target)
		}
		// De-duplicate so a repeated target (e.g. --gather hcp,hcp) does not run twice
		// concurrently into the same output directory.
		if seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no gather targets specified; valid targets are: hcp, mc, sc, sc_acm")
	}

	for _, target := range targets {
		if target == "hcp" && !cluster.Hypershift().Enabled() {
			return nil, fmt.Errorf("--gather hcp requires an HCP cluster ID, but %s is not an HCP cluster; use --gather mc or --gather sc,sc_acm for a management/service-cluster ID", cluster.ID())
		}
	}

	return targets, nil
}

func (mg *mustGather) Run() error {
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	cluster, err := utils.GetClusterAnyStatus(ocmClient, mg.clusterId)
	if err != nil {
		return fmt.Errorf("failed to get OCM cluster info for %s: %s", mg.clusterId, err)
	}

	// Validate targets up front so an unknown target or an incompatible ID/target combination
	// fails before we create output directories or elevate into any cluster.
	gatherTargets, err := parseAndValidateTargets(mg.gatherTargets, cluster)
	if err != nil {
		return err
	}

	// hack(typeid): work around backplane overwriting our config
	err = osdctlConfig.EnsureConfigFile()
	if err != nil {
		return err
	}

	// Prepare for gathering data. Default to the current working directory rather than /tmp, which
	// is RAM-backed (tmpfs) on many systems and would OOM the host for large gathers.
	baseDir := mg.outputDir
	if baseDir == "" {
		baseDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to determine working directory for output: %w", err)
		}
	}

	timestamp := time.Now().Format("20060102150405")
	outputDir := fmt.Sprintf("%s/cluster_dump_%s_%s", baseDir, mg.clusterId, timestamp)
	tarballName := fmt.Sprintf("cluster_dump_%s_%s.tar.gz", mg.clusterId, timestamp)
	outputTarballTmp := fmt.Sprintf("%s/%s", baseDir, tarballName)
	outputTarballPath := fmt.Sprintf("%s/%s", outputDir, tarballName)
	err = os.MkdirAll(outputDir, 0o750)
	if err != nil {
		return err
	}

	// Prints with color :)
	fmt.Printf("\033[1;34mCreating must-gather with targets '%s'. Output directory: '%s'\033[0m\n", strings.Join(gatherTargets, ","), outputDir)

	// Progress tracking; the ticker is stopped deterministically via progressDone once all
	// targets finish, so it never reports after the final summary.
	var completed sync.Map
	totalGatherTargets := len(gatherTargets)
	progressDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		start := time.Now()
		lastCompleted := -1

		for {
			select {
			case <-progressDone:
				return
			case <-ticker.C:
				var completedCount int
				var remaining []string

				for _, gatherType := range gatherTargets {
					if _, ok := completed.Load(gatherType); ok {
						completedCount++
					} else {
						remaining = append(remaining, gatherType)
					}
				}

				// Only reprint when something actually changed, so a single long-running target
				// (e.g. a large HCP log export) does not spam an identical line every tick.
				if completedCount == lastCompleted {
					continue
				}
				lastCompleted = completedCount

				// Prints with color :)
				fmt.Printf("\033[1;34mProgress: %d/%d completed after %s. Remaining: %v\033[0m\n", completedCount, totalGatherTargets, time.Since(start).Round(time.Second), remaining)
			}
		}
	}()

	// Resolve (and elevate into) each infrastructure cluster at most once per invocation.
	// sync.Once makes concurrent targets share a single setup and its error: mc/hcp share the
	// MC, sc/sc_acm share the SC, so we never run the backplane login/elevation twice.
	var (
		mcOnce sync.Once
		mcCfg  *rest.Config
		mcCli  *kubernetes.Clientset
		mcErr  error
	)
	getMC := func() (*rest.Config, *kubernetes.Clientset, error) {
		mcOnce.Do(func() { mcCfg, mcCli, mcErr = resolveMCRestConfig(ocmClient, cluster, mg.reason) })
		return mcCfg, mcCli, mcErr
	}

	var (
		scOnce sync.Once
		scCfg  *rest.Config
		scCli  *kubernetes.Clientset
		scErr  error
	)
	getSC := func() (*rest.Config, *kubernetes.Clientset, error) {
		scOnce.Do(func() { scCfg, scCli, scErr = resolveSCRestConfig(ocmClient, cluster, mg.reason) })
		return scCfg, scCli, scErr
	}

	// Start must-gathers in parallel; each goroutine owns its own result slot (no shared writes).
	results := make([]targetResult, len(gatherTargets))
	var wg sync.WaitGroup
	for i, gatherTarget := range gatherTargets {
		wg.Add(1)
		go func(i int, gatherTarget string) {
			defer wg.Done()
			// Mark this gatherTarget as completed for progress tracking
			defer completed.Store(gatherTarget, true)

			results[i] = mg.runTarget(gatherTarget, outputDir, cluster, ocmClient, getMC, getSC)
		}(i, gatherTarget)
	}

	wg.Wait()
	close(progressDone)

	printGatherSummary(results)

	producedOutput, allSucceeded := summarizeResults(results)

	// Total collection failure: every requested target failed. Do not create an empty archive
	// or print a success message, and exit nonzero.
	if !producedOutput {
		return fmt.Errorf("must-gather failed: all %d requested target(s) failed; no archive created", len(results))
	}

	// We have useful output, so build the archive even when collection was only partial.
	// Prints with color :)
	fmt.Printf("\033[1;34mCompressing collected data into an archive (this can take a while for large gathers)...\033[0m\n")
	if err := createTarball(outputDir, outputTarballTmp); err != nil {
		return fmt.Errorf("failed to create tarball: %w", err)
	}
	if err := os.Rename(outputTarballTmp, outputTarballPath); err != nil {
		return fmt.Errorf("failed to move tarball to output directory: %w", err)
	}

	if allSucceeded {
		fmt.Println("Data collection completed successfully in:", outputDir)
	} else {
		// Prints with color :)
		fmt.Printf("\033[1;33mWARNING: must-gather completed with PARTIAL results - some targets or substeps failed (see summary above). Collected output has been retained.\033[0m\n")
	}
	fmt.Println("Compressed archive has been created at:", outputTarballPath)

	return nil
}

type gatherStatus string

const (
	statusSucceeded gatherStatus = "SUCCEEDED"
	statusPartial   gatherStatus = "PARTIAL"
	statusFailed    gatherStatus = "FAILED"
)

// targetResult records the outcome of a single gather target for the final summary and the
// archive/exit decision.
type targetResult struct {
	target   string
	role     string
	artifact string
	status   gatherStatus
	failures []string
	note     string // optional extra info for the summary (e.g. RHOBS record counts)
}

func (r targetResult) fail(reason string) targetResult {
	r.status = statusFailed
	r.failures = append(r.failures, reason)
	return r
}

func (r targetResult) succeed() targetResult {
	r.status = statusSucceeded
	return r
}

// runTarget executes a single gather target and reports its outcome, artifact location and any
// substep failures. The hcp target tracks the RHOBS log export and the ACM dump as independent
// substeps (see runHCPTarget).
func (mg *mustGather) runTarget(target, outputDir string, cluster *cmv1.Cluster, ocmClient *ocmsdk.Connection, getMC, getSC func() (*rest.Config, *kubernetes.Clientset, error)) targetResult {
	switch target {
	case "sc":
		return mg.runInfraTarget(target, "service cluster infrastructure", outputDir+"/sc_infra", getSC)
	case "sc_acm":
		return mg.runInfraTarget(target, "service cluster ACM", outputDir+"/sc_acm", getSC, "--image="+mg.acmMustGatherImage)
	case "mc":
		return mg.runInfraTarget(target, "management cluster infrastructure", outputDir+"/mc_infra", getMC)
	case "hcp":
		return mg.runHCPTarget(outputDir, cluster, ocmClient, getMC)
	default:
		return targetResult{target: target}.fail(fmt.Sprintf("unknown gather target %q", target))
	}
}

// runInfraTarget resolves a cluster via getConfig and runs a plain `oc adm must-gather` into
// artifact (with any extra flags, e.g. a custom --image). It backs the sc/sc_acm/mc targets, which
// differ only in cluster, artifact dir and flags.
func (mg *mustGather) runInfraTarget(target, role, artifact string, getConfig func() (*rest.Config, *kubernetes.Clientset, error), extraArgs ...string) targetResult {
	res := targetResult{target: target, role: role, artifact: artifact}
	cfg, cli, err := getConfig()
	if err != nil {
		return res.fail(fmt.Sprintf("resolve cluster: %v", err))
	}
	if err := mg.runMustGather(cfg, cli, append([]string{"--dest-dir=" + artifact}, extraArgs...)); err != nil {
		return res.fail(fmt.Sprintf("must-gather: %v", err))
	}
	return res.succeed()
}

// runHCPTarget collects the HCP control-plane: RHOBS logs/events and the ACM hypershift dump are
// independent substeps, so one failing while the other succeeds yields a PARTIAL result.
func (mg *mustGather) runHCPTarget(outputDir string, cluster *cmv1.Cluster, ocmClient *ocmsdk.Connection, getMC func() (*rest.Config, *kubernetes.Clientset, error)) targetResult {
	res := targetResult{target: "hcp", role: "HCP control-plane (RHOBS logs + ACM hypershift dump)", artifact: outputDir + "/hcp"}
	destDir := res.artifact

	// Resolve the HCP namespaces once; both substeps depend on them, so a failure here fails both.
	clusterHyperShift, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(mg.clusterId).Hypershift().Get().Send()
	if err != nil {
		return res.fail(fmt.Sprintf("get hypershift info for %s: %v", mg.clusterId, err))
	}
	hcpNamespace, ok := clusterHyperShift.Body().GetHCPNamespace()
	if !ok {
		return res.fail("get HCP namespace")
	}
	hcName := cluster.DomainPrefix()
	hcNamespace := strings.TrimSuffix(hcpNamespace, "-"+hcName)

	// Substep 1: gather logs and events from RHOBS (replaces the former Dynatrace collection).
	logsOK := true
	rhobsNamespaces := []string{
		hcpNamespace,
		"klusterlet-" + cluster.ID(),
		hcNamespace,
		"hypershift",
		"cert-manager",
		"redhat-cert-manager-operator",
		"open-cluster-management-agent",
		"open-cluster-management-agent-addon",
	}
	logCount, err := mg.gatherHCPLogs(context.Background(), mg.clusterId, mg.hiveOcmUrl, destDir, rhobsNamespaces, 72*time.Hour, mg.maxLogLines)
	if err != nil {
		logsOK = false
		res.failures = append(res.failures, fmt.Sprintf("RHOBS log export: %v", err))
	}
	res.note = fmt.Sprintf("RHOBS: %d log lines across %d namespaces", logCount, len(rhobsNamespaces))

	// Substep 2: ACM must-gather which includes running the hypershift binary for a dump.
	dumpOK := true
	mcCfg, mcCli, err := getMC()
	if err != nil {
		dumpOK = false
		res.failures = append(res.failures, fmt.Sprintf("resolve MC for ACM dump: %v", err))
	} else {
		// The gather command and its args must follow a "--" separator and be passed as separate
		// argv elements; otherwise `oc adm must-gather` ignores them and runs the image's default
		// entrypoint (which does not perform the hosted-cluster hypershift dump).
		gatherArgs := []string{
			"--dest-dir=" + destDir,
			"--image=" + mg.acmMustGatherImage,
			"--",
			"/usr/bin/gather",
			"hosted-cluster-namespace=" + hcNamespace,
			"hosted-cluster-name=" + hcName,
		}
		if err := mg.runMustGather(mcCfg, mcCli, gatherArgs); err != nil {
			dumpOK = false
			res.failures = append(res.failures, fmt.Sprintf("ACM hypershift dump: %v", err))
		}
	}

	res.status = combineSubsteps(logsOK, dumpOK)
	return res
}

// combineSubsteps reduces substep outcomes into a target status: all ok is SUCCEEDED, none ok is
// FAILED, and a mix is PARTIAL.
func combineSubsteps(oks ...bool) gatherStatus {
	all, any := true, false
	for _, ok := range oks {
		all = all && ok
		any = any || ok
	}
	switch {
	case all:
		return statusSucceeded
	case any:
		return statusPartial
	default:
		return statusFailed
	}
}

// summarizeResults reports whether any target produced useful output (succeeded or partial) and
// whether every target fully succeeded, driving the archive and exit decisions.
func summarizeResults(results []targetResult) (producedOutput, allSucceeded bool) {
	allSucceeded = true
	for _, result := range results {
		switch result.status {
		case statusSucceeded:
			producedOutput = true
		case statusPartial:
			producedOutput = true
			allSucceeded = false
		default:
			allSucceeded = false
		}
	}
	return producedOutput, allSucceeded
}

func printGatherSummary(results []targetResult) {
	fmt.Println()
	fmt.Println("Must-gather summary:")
	for _, result := range results {
		fmt.Printf("  [%s] %s - %s (artifacts: %s)\n", result.status, result.target, result.role, result.artifact)
		if result.note != "" {
			fmt.Printf("      %s\n", result.note)
		}
		for _, failure := range result.failures {
			fmt.Printf("      ! %s\n", failure)
		}
	}
}

// resolveClusterRestConfig returns the REST config and k8s client for an infrastructure cluster.
// For an HCP cluster ID it resolves the parent MC/SC via resolve; for a direct MC/SC ID it uses the
// cluster as-is. role ("management"/"service") only labels the error messages.
func resolveClusterRestConfig(ocmClient *ocmsdk.Connection, cluster *cmv1.Cluster, reason, role string,
	resolve func(context.Context, string, *ocmsdk.Connection) (*cmv1.Cluster, error)) (*rest.Config, *kubernetes.Clientset, error) {
	id := cluster.ID()
	if cluster.Hypershift().Enabled() {
		infra, err := resolve(context.Background(), cluster.ID(), ocmClient)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to resolve %s cluster for %s: %w", role, cluster.ID(), err)
		}
		id = infra.ID()
	}

	_, restCfg, k8sCli, err := common.GetKubeConfigAndClientWithConn(id, ocmClient, reason)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get kubeconfig for %s cluster %s: %w", role, id, err)
	}
	return restCfg, k8sCli, nil
}

func resolveMCRestConfig(ocmClient *ocmsdk.Connection, cluster *cmv1.Cluster, reason string) (*rest.Config, *kubernetes.Clientset, error) {
	return resolveClusterRestConfig(ocmClient, cluster, reason, "management", utils.GetManagementClusterWithConn)
}

func resolveSCRestConfig(ocmClient *ocmsdk.Connection, cluster *cmv1.Cluster, reason string) (*rest.Config, *kubernetes.Clientset, error) {
	return resolveClusterRestConfig(ocmClient, cluster, reason, "service", utils.GetServiceClusterWithConn)
}

func createMustGather(restCfg *rest.Config, k8sCli *kubernetes.Clientset, additionalFlags []string) error {
	// We used to run this programatically by directly using the must-gather package  (see https://github.com/openshift/osdctl/pull/660)
	// from the oc cli, but decided to opt for oc.Exec instead.
	// Reasoning:
	// 1) the must-gather internals, even when called programmatically, already shelled out to `oc rsync`.
	// We had to hack around this by overriding the shell out with a specific kubeconfig.
	// 2) the vendored dependencies of oc were causing issues with `go list`, see https://github.com/openshift/osdctl/pull/665.
	kubeConfigFile := createKubeconfigFileForRestConfig(restCfg)
	defer os.Remove(kubeConfigFile)

	// Handle sigints and sigterms
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("Received interrupt signal, canceling operation...")
		cancel()
	}()

	cmdArgs := []string{"adm", "must-gather", "--kubeconfig=" + kubeConfigFile}
	cmdArgs = append(cmdArgs, additionalFlags...)

	cmd := exec.CommandContext(ctx, "oc", cmdArgs...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		if ctx.Err() == context.Canceled {
			return fmt.Errorf("command was canceled by user (e.g., Ctrl+C): %v\nstderr: %s", err, stderr.String())
		}
		return fmt.Errorf("failed to run 'oc adm must-gather': %v\nstderr: %s", err, stderr.String())
	}

	return nil
}

// The sole purpose of this function is to work around the hack described in `createMustGather`
func createKubeconfigFileForRestConfig(restConfig *rest.Config) string {
	var proxyUrl *url.URL
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	if restConfig.Proxy != nil {
		proxyUrl, _ = restConfig.Proxy(req)
	}

	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   restConfig.Host,
		CertificateAuthorityData: restConfig.CAData,
		ProxyURL:                 proxyUrl.String(),
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:  "default-cluster",
		AuthInfo: "default-user",
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo)

	authinfos["default-user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: restConfig.CertData,
		ClientKeyData:         restConfig.KeyData,
		Impersonate:           restConfig.Impersonate.UserName,
		Token:                 restConfig.BearerToken,
	}

	val, ok := restConfig.Impersonate.Extra["reason"]
	if ok {
		impersonateUserExtra := make(map[string][]string)
		impersonateUserExtra["reason"] = val
		authinfos["default-user"].ImpersonateUserExtra = impersonateUserExtra
	}

	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}

	kubeConfigFile, _ := os.CreateTemp("", "kubeconfig")
	_ = clientcmd.WriteToFile(clientConfig, kubeConfigFile.Name())
	return kubeConfigFile.Name()
}

func createTarball(sourceDir, tarballName string) error {
	tarballFile, err := os.Create(tarballName)
	if err != nil {
		return fmt.Errorf("failed to create tarball file: %v", err)
	}
	defer tarballFile.Close()

	gzipWriter := gzip.NewWriter(tarballFile)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Walk through the directory and add files to the tarball
	err = filepath.Walk(sourceDir, func(file string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking the path %v: %v", file, err)
		}

		// Skip the root directory itself
		if file == sourceDir {
			return nil
		}

		// Create the header for the file entry
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("failed to create tar header for file %v: %v", file, err)
		}

		// Set the relative name for the file in the tarball (strip the sourceDir prefix)
		relPath, err := filepath.Rel(sourceDir, file)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %v", err)
		}
		header.Name = relPath

		// Write the header for the file into the tarball
		err = tarWriter.WriteHeader(header)
		if err != nil {
			return fmt.Errorf("failed to write header for file %v: %v", file, err)
		}

		// If it's a regular file, add its content to the tarball
		if !info.IsDir() {
			fileToArchive, err := os.Open(file)
			if err != nil {
				return fmt.Errorf("failed to open file %v: %v", file, err)
			}
			defer fileToArchive.Close()

			// Copy the file content into the tarball
			_, err = io.Copy(tarWriter, fileToArchive)
			if err != nil {
				return fmt.Errorf("failed to write file content for file %v: %v", file, err)
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("error walking source directory: %v", err)
	}

	return nil
}
