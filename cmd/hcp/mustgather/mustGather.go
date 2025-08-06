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

	"github.com/openshift/osdctl/cmd/common"
	"github.com/openshift/osdctl/cmd/dynatrace"
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
}

func NewCmdMustGather() *cobra.Command {
	mg := &mustGather{}

	mustGatherCommand := &cobra.Command{
		Use:     "must-gather --cluster-id <cluster-identifier>",
		Short:   "Create a must-gather for HCP cluster",
		Long:    "Create a must-gather for an HCP cluster with optional gather targets",
		Example: "osdctl hcp must-gather --cluster-id CLUSTER_ID --gather sc,mc,sc_acm --reason OHSS-1234",
		RunE: func(cmd *cobra.Command, args []string) error {

			return mg.Run()
		},
	}

	defaultAcmImage := "quay.io/stolostron/must-gather:2.11.4-SNAPSHOT-2024-12-02-15-19-44"
	mustGatherCommand.Flags().StringVarP(&mg.clusterId, "cluster-id", "C", "", "Internal ID of the cluster to gather data from")
	mustGatherCommand.Flags().StringVar(&mg.reason, "reason", "", "The reason for this command, which requires elevation (e.g., OHSS ticket or PD incident).")
	mustGatherCommand.Flags().StringVar(&mg.gatherTargets, "gather", "hcp", "Comma-separated list of gather targets (available: sc, sc_acm, mc, hcp).")
	mustGatherCommand.Flags().StringVar(&mg.acmMustGatherImage, "acm_image", defaultAcmImage, "Overrides the acm must-gather image being used for acm mc, sc as well as hcp must-gathers.")

	mustGatherCommand.MarkFlagRequired("cluster-id")
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
	err = os.MkdirAll(outputDir, 0750)
	if err != nil {
		return err
	}

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
			// Mark this gatherTarget as completed for progress tracking
			defer completed.Store(gatherTarget, true)

			switch gatherTarget {
			case "sc":
				destDir := outputDir + "/sc_infra"
				if err := createMustGather(scRestCfg, scK8sCli, []string{"--dest-dir=" + destDir}); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "sc_acm":
				destDir := outputDir + "/sc_acm"
				if err := createMustGather(scRestCfg, scK8sCli, []string{"--dest-dir=" + destDir, "--image=" + mg.acmMustGatherImage}); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "mc":
				destDir := outputDir + "/mc_infra"
				if err := createMustGather(mcRestCfg, mcK8sCli, []string{"--dest-dir=" + destDir}); err != nil {
					fmt.Printf("failed to gather %s: %v\n", gatherTarget, err)
				}
			case "hcp":
				destDir := outputDir + "/hcp"

				// 1. Gather logs from DT
				gatherOptions := &dynatrace.GatherLogsOpts{Since: 72, SortOrder: "asc", DestDir: destDir}
				if err := gatherOptions.GatherLogs(mg.clusterId); err != nil {
					fmt.Printf("failed to gather HCP dynatrace logs: %v\n", err)
				}

				// 2. ACM must-gather which includes running the hypershift binary for a dump
				clusterHyperShift, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(mg.clusterId).Hypershift().Get().Send()
				if err != nil {
					fmt.Printf("collected HCP dynatrace logs but failed to get OCM cluster hypershift info for %s: %v\n", mg.clusterId, err)
					return
				}

				hcpNamespace, ok := clusterHyperShift.Body().GetHCPNamespace()
				if !ok {
					fmt.Println("collected HCP dynatrace logs but failed to get HCP namespace")
					return
				}

				hcName := cluster.DomainPrefix()
				hcNamespace := strings.TrimSuffix(hcpNamespace, "-"+hcName)

				// TODO(ACM-16170): replace this with an official ACM release image once it's available
				acmHyperShiftImage := "quay.io/rokejungrh/must-gather:v2.13.0-33-linux"
				gatherScript := fmt.Sprintf("/usr/bin/gather hosted-cluster-namespace=%s hosted-cluster-name=%s", hcNamespace, hcName)
				if err := createMustGather(mcRestCfg, mcK8sCli, []string{"--dest-dir=" + destDir, "--image=" + acmHyperShiftImage, gatherScript}); err != nil {
					fmt.Printf("collected HCP dynatrace logs but failed to gather %s: %v\n", gatherTarget, err.Error())
				}

			default:
				fmt.Printf("unknown gather type: %s\n", gatherTarget)
			}
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
