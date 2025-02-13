package mustgather

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	configclient "github.com/openshift/client-go/config/clientset/versioned"
	imagev1client "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	"github.com/openshift/oc/pkg/cli/admin/mustgather"
	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/dynatrace"
	"github.com/openshift/osdctl/pkg/osdctlConfig"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	"k8s.io/cli-runtime/pkg/printers"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	kcmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/scheme"
)

var (
	acmMustGatherImage = "quay.io/stolostron/must-gather:2.11.4-SNAPSHOT-2024-12-02-15-19-44"
)

type mustGather struct {
	clusterId     string
	reason        string
	gatherTargets string
}

func NewCmdMustGather() *cobra.Command {
	mg := &mustGather{}

	mustGatherCommand := &cobra.Command{
		Use:     "must-gather",
		Short:   "Create a must-gather for HCP cluster",
		Long:    "Create a must-gather for an HCP cluster with optional gather targets",
		Example: "osdctl hcp must-gather $CLUSTER_ID --gather sc_mg,mc_mg,sc_acm --reason OHSS-1234",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mg.clusterId = args[0]

			return mg.Run()
		},
	}

	mustGatherCommand.Flags().StringVar(&mg.reason, "reason", "", "The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).")
	mustGatherCommand.Flags().StringVar(&mg.gatherTargets, "gather", "hcp", "Comma-separated list of gather targets (available: sc_mg, sc_acm, mc_mg, mc_acm, hc, hcp).")

	mustGatherCommand.MarkFlagRequired("reason")

	return mustGatherCommand
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

	mc, err := utils.GetManagementCluster(cluster.ID())
	if err != nil {
		return err
	}

	sc, err := utils.GetServiceCluster(cluster.ID())
	if err != nil {
		return err
	}

	_, hcRestCfg, hcK8sCli, err := common.GetKubeConfigAndClient(cluster.ID(), mg.reason)
	if err != nil {
		return err
	}

	_, mcRestCfg, mcK8sCli, err := common.GetKubeConfigAndClient(mc.ID(), mg.reason)
	if err != nil {
		return err
	}

	_, scRestCfg, scK8sCli, err := common.GetKubeConfigAndClient(sc.ID(), mg.reason)
	if err != nil {
		return err
	}

	// hack(typeid): work around backplane overwriting our config
	err = osdctlConfig.EnsureConfigFile()
	if err != nil {
		return err
	}

	// Prepare for gathering data
	timestamp := time.Now().Format("20060102150405")
	baseDir := "/tmp"
	outputDir := fmt.Sprintf("%s/cluster_dump_%s_%s", baseDir, mg.clusterId, timestamp)
	tarballName := fmt.Sprintf("cluster_dump_%s_%s.tar.gz", mg.clusterId, timestamp)
	outputTarballTmp := fmt.Sprintf("%s/%s", baseDir, tarballName)
	outputTarballPath := fmt.Sprintf("%s/%s", outputDir, tarballName)
	os.MkdirAll(outputDir, os.ModePerm)

	// Prints with color :)
	fmt.Printf("\033[1;34mCreating must-gather with targets '%s'. Output directory: '%s'\033[0m\n", mg.gatherTargets, outputDir)
	gatherTargets := strings.Split(mg.gatherTargets, ",")

	// Progress tracking
	var completed sync.Map
	var totalGatherTargets = len(gatherTargets)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			var completedCount int
			var remaining []string

			for _, gatherType := range gatherTargets {
				if _, ok := completed.Load(gatherType); ok {
					completedCount++
				} else {
					remaining = append(remaining, gatherType)
				}
			}

			// Prints with color :)
			fmt.Printf("\033[1;34mProgress: %d/%d completed. Remaining: %v\033[0m\n", completedCount, totalGatherTargets, remaining)

			if completedCount == totalGatherTargets {
				return
			}
		}
	}()

	// Start must-gathers in parallel
	var wg sync.WaitGroup
	for _, gatherTarget := range gatherTargets {
		wg.Add(1)
		go func(gatherTarget string) {
			defer wg.Done()

			switch gatherTarget {
			case "sc_mg":
				if err := createMustGather(scRestCfg, scK8sCli, outputDir+"/sc_infra", ""); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "sc_acm":
				if err := createMustGather(scRestCfg, scK8sCli, outputDir+"/sc_acm", acmMustGatherImage); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "mc_mg":
				if err := createMustGather(mcRestCfg, mcK8sCli, outputDir+"/mc_infra", ""); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "mc_acm":
				if err := createMustGather(mcRestCfg, mcK8sCli, outputDir+"/mc_acm", acmMustGatherImage); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "hcp":
				gatherOptions := &dynatrace.GatherLogsOpts{Since: 72, SortOrder: "asc", DestDir: outputDir + "/hcp_logs_dump"}
				if err := gatherOptions.GatherLogs(mg.clusterId); err != nil {
					fmt.Printf("failed to gather HCP dynatrace logs: %v\n", err)
				}
			case "hc":
				if err := createMustGather(hcRestCfg, hcK8sCli, outputDir+"/hc", ""); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			default:
				fmt.Printf("unknown gather type: %s\n", gatherTarget)
			}

			// Mark this gatherTarget as completed for progress tracking
			completed.Store(gatherTarget, true)
		}(gatherTarget)
	}

	wg.Wait()
	fmt.Println()
	fmt.Println("All must-gather tasks completed. Creating tarball.")

	// Create a tarball with all collected data
	if err := createTarball(outputDir, outputTarballTmp); err != nil {
		return fmt.Errorf("failed to create tarball: %w", err)
	}

	// Move the tarball we create from collected data back in the output directory
	err = os.Rename(outputTarballTmp, outputTarballPath)
	if err != nil {
		return fmt.Errorf("failed to move tarball to output directory: %w", err)
	}

	fmt.Println("Data collection completed successfully in:", outputDir)
	fmt.Println("Compressed archive has been created at:", outputTarballPath)

	return nil
}

func createMustGather(restCfg *rest.Config, k8sCli *kubernetes.Clientset, destDir, image string) error {
	// hack(typeid): there's two major issues with using the oc cli's must gather:
	// 1) oc cli assumes the client used for rsh and will default to a "sibling" command.
	// This results in oc's rsync (used to copy files from a must-gather pod) to used `oc rsh`
	// See here: https://github.com/openshift/oc/blob/cbc8edcc9c58cd8b2238d01e55ae8c8b0979ec4e/pkg/cli/rsync/copy_rsync.go#L35
	// In our case, the default rsync remote shell gets resolved to standard rsh, which does not work as we are passing
	// parameters like --namespace!
	// 2) oc cli shells out to call oc rsh (in our case, just rsh) and assumes the kubeconfig.
	// As we are running must gathers for multiple clusters, we need to make sure we're using the right kubeconfigs
	// that we created programmatically, vs the one we currently have as a file locally.
	// See here: https://github.com/openshift/oc/blob/cbc8edcc9c58cd8b2238d01e55ae8c8b0979ec4e/pkg/cli/rsync/copy_rsync.go#L84
	// WORKAROUND:
	// [1] we are overriding the `o.RsyncRshCmd` to use `oc rsh` vs `rsh`.
	// For this, we need to use `mustgather.NewGatherOptions()` vs `mustgather.NewGatherCommand`, which adds a lot of bloat code.
	// [2] we are passing a kubeconfig parameter, for that, we create a temporary kubeconfig from retrieved restConfig.

	// We're not printing those out, as the logs are in the must-gather output.
	var stdout, stderr bytes.Buffer
	streams := genericiooptions.IOStreams{
		In:     nil,
		Out:    &stdout,
		ErrOut: &stderr,
	}

	kubeConfigFile := createKubeconfigFileForRestConfig(restCfg)
	defer os.Remove(kubeConfigFile)

	o := mustgather.NewMustGatherOptions(streams)
	o.Client = k8sCli
	o.Images = []string{image}
	o.DestDir = destDir
	o.RsyncRshCmd = fmt.Sprintf("oc rsh --kubeconfig=%s", kubeConfigFile)
	o.Config = restCfg
	flags := genericclioptions.NewConfigFlags(false)
	o.RESTClientGetter = kcmdutil.NewFactory(flags)

	var err error
	if o.ConfigClient, err = configclient.NewForConfig(restCfg); err != nil {
		return err
	}
	if o.DynamicClient, err = dynamic.NewForConfig(restCfg); err != nil {
		return err
	}
	if o.ImageClient, err = imagev1client.NewForConfig(restCfg); err != nil {
		return err
	}

	o.PrinterCreated, err = printers.NewTypeSetter(scheme.Scheme).WrapToPrinter(&printers.NamePrinter{Operation: "created"}, nil)
	if err != nil {
		return err
	}
	o.PrinterDeleted, err = printers.NewTypeSetter(scheme.Scheme).WrapToPrinter(&printers.NamePrinter{Operation: "deleted"}, nil)
	if err != nil {
		return err
	}

	if err := o.Run(); err != nil {
		return err
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
