package hive

import (
	"context"
	"fmt"
	"os"
	"strings"

	ocmConfig "github.com/openshift-online/ocm-common/pkg/ocm/config"
	sdk "github.com/openshift-online/ocm-sdk-go"
	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	configv1 "github.com/openshift/api/config/v1"
	hivev1 "github.com/openshift/hive/apis/hive/v1"
	common "github.com/openshift/osdctl/cmd/common"
	k8s "github.com/openshift/osdctl/pkg/k8s"
	"github.com/openshift/osdctl/pkg/printer"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// testHiveLoginOptions defines the struct for running ocm and backplane client tests

type testHiveLoginOptions struct {
	clusterID         string
	verbose           bool
	hiveOcmConfigPath string
	hiveOcmURL        string
	reason            string
}

const longDescription = `
This test utility attempts to exercise and validate OSDCTL's functions related to
OCM and backplane client connections. 
	
This test utility can be run against an OSD/Rosa Classic target cluster. This utility
will attempt to discover the Hive cluster, and create both
OCM and kube client connections, and perform basic requests for each to connection in 
order to validate functionality of the related OSDCTL utility functions.  
	
This test utility allows for the target cluster to exist in a separate OCM 
environment (ie integration, staging) from the hive cluster (ie production).

The default OCM environment vars should be set for the target cluster. 
If the target cluster exists outside of the OCM 'production' environment, the user 
has the option to provide the production OCM config (with valid token set), 
or provide the production OCM API url as a command argument, or set the value in the osdctl 
config yaml file (ie: "hive_ocm_url: https://api.openshift.com" or "hive_ocm_url: production" ).
For testing purposes comment out 'hive_ocm_url' from osdctl's config if testing an empty value. 
`

// Defines command to run through series of tests to validate existing and new and legacy
// ocm + backplane client functions
func newCmdTestHiveLogin() *cobra.Command {
	ops := newtestHiveLoginOptions()
	testHiveLoginCmd := &cobra.Command{
		Use:               "login-tests",
		Short:             "Test utility to exercise OSDCTL client connections for both Target Cluster and it's Hive Cluster.",
		Long:              longDescription,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.run())
		},
	}

	testHiveLoginCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	testHiveLoginCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	testHiveLoginCmd.Flags().StringVar(&ops.hiveOcmConfigPath, "hive-ocm-config", "", "OCM config for hive if different than Cluster")
	testHiveLoginCmd.Flags().StringVar(&ops.hiveOcmURL, "hive-ocm-url", "", "OCM URL for hive, this will fallback to reading from the osdctl config value: 'hive_ocm_url' if left empty")

	_ = testHiveLoginCmd.MarkFlagRequired("cluster-id")
	return testHiveLoginCmd
}

func newtestHiveLoginOptions() *testHiveLoginOptions {
	return &testHiveLoginOptions{}
}

func printDiv() {
	fmt.Printf("\n---------------------------------------------------------------\n\n")
}

func dumpClusterOperators(kubeClient client.Client) error {
	var cos configv1.ClusterOperatorList
	if err := kubeClient.List(context.TODO(), &cos, &client.ListOptions{}); err != nil {
		fmt.Fprintf(os.Stderr, "error fetching cluster operators, err:'%s'\n", err)
		return err
	}
	table := printer.NewTablePrinter(os.Stderr, 20, 1, 1, ' ')
	table.AddRow([]string{"NAME", "AVAILABLE", "PROGRESSING", "DEGRADED"})
	for _, co := range cos.Items {
		var available configv1.ConditionStatus
		var progressing configv1.ConditionStatus
		var degraded configv1.ConditionStatus
		for _, cond := range co.Status.Conditions {
			switch cond.Type {
			case configv1.OperatorAvailable:
				available = cond.Status
			case configv1.OperatorProgressing:
				progressing = cond.Status
			case configv1.OperatorDegraded:
				degraded = cond.Status
			}
		}
		table.AddRow([]string{co.Name, string(available), string(progressing), string(degraded)})
	}
	table.Flush()
	return nil
}

func getClusterDeployment(hiveKubeClient client.Client, clusterID string) (cd *hivev1.ClusterDeployment, err error) {
	var cds hivev1.ClusterDeploymentList
	if err := hiveKubeClient.List(context.TODO(), &cds, &client.ListOptions{}); err != nil {
		fmt.Printf("err fetching cluster deployments, err:'%v'", err)
		return nil, err
	}
	for _, cdeploy := range cds.Items {
		if strings.Contains(cdeploy.Namespace, clusterID) {
			fmt.Printf("Got Hive ClusterDeployment for target cluster:'%s'\n", cdeploy.Name)
			return &cdeploy, nil
		}
	}
	return nil, fmt.Errorf("clusterDeployment for cluster:'%s' not found", clusterID)
}

// setupOCMConnection creates an OCM client and fetches the target cluster
func setupOCMConnection(clusterID string) (*sdk.Connection, *v1.Cluster, string, error) {
	printDiv()
	fmt.Printf("Building ocm client using legacy functions and env vars...\n")
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return nil, nil, "", err
	}

	cluster, err := utils.GetClusterAnyStatus(ocmClient, clusterID)
	if err != nil {
		fmt.Printf("Failed to fetch cluster '%s' from OCM, err:'%v'", clusterID, err)
		return nil, nil, "", err
	}

	actualClusterID := cluster.ID()
	if clusterID != actualClusterID {
		fmt.Printf("Using internal ID:'%s' for provided cluster:'%s'\n", actualClusterID, clusterID)
	}

	fmt.Printf("Fetched cluster from OCM:'%s'\n", actualClusterID)
	printDiv()

	return ocmClient, cluster, actualClusterID, nil
}

// setupHiveOCMConfig builds the Hive OCM configuration from provided options
func setupHiveOCMConfig(hiveOcmConfigPath, hiveOcmURL string) (*ocmConfig.Config, error) {
	var hiveOCMCfg *ocmConfig.Config
	var err error

	// Test building OCM config from a provided file path
	if len(hiveOcmConfigPath) > 0 {
		fmt.Printf("Attempting to build OCM config from provided file path...\n")
		hiveOCMCfg, err = utils.GetOcmConfigFromFilePath(hiveOcmConfigPath)
		if err != nil {
			fmt.Printf("Failed to build Hive OCM config from file path:'%s'\n", hiveOcmConfigPath)
			return nil, err
		}
	}

	// Test replacing just the OCM URL for an already built config
	if len(hiveOcmURL) > 0 {
		if hiveOCMCfg == nil {
			fmt.Printf("Attempting to build OCM config...\n")
			hiveOCMCfg, err = utils.GetOCMConfigFromEnv()
			if err != nil {
				fmt.Printf("Failed to build OCM config from legacy function\n")
				return nil, err
			}
		}
		hiveOCMCfg.URL = hiveOcmURL
	}

	return hiveOCMCfg, nil
}

// setupHiveOCMConnection creates an OCM connection for Hive using the provided config
func setupHiveOCMConnection(hiveOCMCfg *ocmConfig.Config, hiveOcmConfigPath string) (*sdk.Connection, error) {
	if hiveOCMCfg == nil {
		return nil, nil
	}

	hiveBuilder, err := utils.GetOCMSdkConnBuilderFromConfig(hiveOCMCfg)
	if err != nil {
		fmt.Printf("Failed to create sdk connection builder from hive ocm cfg, err:'%s'\n", err)
		return nil, err
	}

	hiveOCM, err := hiveBuilder.Build()
	if err != nil {
		fmt.Printf("Error connecting to OCM env using config at: '%s'\nErr:%v", hiveOcmConfigPath, err)
		return nil, err
	}

	fmt.Printf("Built OCM config and connection from provided config inputs\n")
	printDiv()

	return hiveOCM, nil
}

// setupHiveCluster fetches the Hive cluster for the target cluster
func setupHiveCluster(clusterID string, ocmClient, hiveOCM *sdk.Connection) (*v1.Cluster, error) {
	// No OCM related config provided, test the legacy path
	if hiveOCM == nil {
		fmt.Println("---- No hive config provided. Using same OCM connections for target cluster and hive ----")
		hiveOCM = ocmClient
		_, err := utils.GetHiveCluster(clusterID)
		if err != nil {
			fmt.Printf("Failed to fetch hive cluster from OCM with legacy function, err:'%v'", err)
			return nil, err
		}
	}

	printDiv()
	hiveCluster, err := utils.GetHiveClusterWithConn(clusterID, ocmClient, hiveOCM)
	if err != nil {
		fmt.Printf("Failed to fetch hive cluster with provided OCM connection, err:'%v'", err)
		return nil, err
	}

	fmt.Printf("Got Hive Cluster from OCM:'%s'\n", hiveCluster.ID())
	printDiv()

	return hiveCluster, nil
}

// testK8sNew tests creating a Kube client using k8s.New()
func testK8sNew(clusterID string, cluster *v1.Cluster) error {
	fmt.Println("Attempting to create and test Kube Client with k8s.New()...")
	kubeClient, err := k8s.New(clusterID, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to login to cluster:'%s', err: %w", clusterID, err)
	}
	fmt.Printf("Created client connection to target cluster:'%s', '%s'\n", cluster.ID(), cluster.Name())

	if err := dumpClusterOperators(kubeClient); err != nil {
		return err
	}

	fmt.Println("Create and test Kube Client with k8s.New() - PASS")
	printDiv()
	return nil
}

// testK8sNewWithConn tests creating a Kube client using k8s.NewWithConn()
func testK8sNewWithConn(hiveCluster *v1.Cluster, hiveOCM *sdk.Connection) error {
	fmt.Println("Attempting to create and test Kube Client with k8s.NewWithConn()...")
	hiveClient, err := k8s.NewWithConn(hiveCluster.ID(), client.Options{}, hiveOCM)
	if err != nil {
		return fmt.Errorf("failed to login to hive cluster:'%s', err %w", hiveCluster.ID(), err)
	}
	fmt.Printf("Created client connection to HIVE cluster:'%s', '%s'\n", hiveCluster.ID(), hiveCluster.Name())

	if err := dumpClusterOperators(hiveClient); err != nil {
		return err
	}

	fmt.Println("Create and test Kube Client with k8s.NewWithConn() - PASS")
	printDiv()
	return nil
}

// testK8sNewAsBackplaneClusterAdmin tests creating an elevated Kube client using k8s.NewAsBackplaneClusterAdminWithConn()
func testK8sNewAsBackplaneClusterAdmin(hiveCluster *v1.Cluster, hiveOCM *sdk.Connection, clusterID, reason string) error {
	fmt.Println("Attempting to create and test Kube Client with k8s.NewAsBackplaneClusterAdminWithConn()...")
	hiveAdminClient, err := k8s.NewAsBackplaneClusterAdminWithConn(hiveCluster.ID(), client.Options{}, hiveOCM, reason)
	if err != nil {
		return fmt.Errorf("failed to login to hive cluster:'%s', err %w", hiveCluster.ID(), err)
	}
	fmt.Printf("Created 'ClusterAdmin' client connection to HIVE cluster:'%s', '%s'\n", hiveCluster.ID(), hiveCluster.Name())

	clusterDep, err := getClusterDeployment(hiveAdminClient, clusterID)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched ClusterDeployment:'%s/%s' for cluster:'%s' from HIVE using elevated client\n", clusterDep.Namespace, clusterDep.Name, clusterID)
	fmt.Println("Create and test Kube Client withk8s.NewAsBackplaneClusterAdminWithConn() - PASS")
	printDiv()
	return nil
}

// testGetKubeConfigAndClient tests GetKubeConfigAndClient() without admin elevation
func testGetKubeConfigAndClient(clusterID string) error {
	fmt.Printf("Testing non-backplane-admin client, clientSet GetKubeConfigAndClient() for cluster:'%s'\n", clusterID)
	kubeCli, _, kubeClientSet, err := common.GetKubeConfigAndClient(clusterID)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(kubeCli); err != nil {
		return err
	}

	nsList, err := kubeClientSet.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("ClientSet list namespaces failed, err:'%v'\n", err)
		return err
	}

	fmt.Printf("Got '%d' namespaces\n", len(nsList.Items))
	fmt.Println("non-bpadmin Create and test Kube Client, Clientset with GetKubeConfigAndClient() - PASS")
	printDiv()
	return nil
}

// testGetKubeConfigAndClientWithConn tests GetKubeConfigAndClientWithConn() without admin elevation
func testGetKubeConfigAndClientWithConn(clusterID string, ocmClient *sdk.Connection) error {
	fmt.Printf("Testing non-backplane-admin client, clientset GetKubeConfigAndClientWithConn for cluster:'%s'\n", clusterID)
	kubeCli, _, kubeClientSet, err := common.GetKubeConfigAndClientWithConn(clusterID, ocmClient)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(kubeCli); err != nil {
		return err
	}

	nsList, err := kubeClientSet.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("ClientSet list namespaces failed, err:'%v'\n", err)
		return err
	}

	fmt.Printf("Got '%d' namespaces\n", len(nsList.Items))
	fmt.Println("non-bpadmin Create and test Kube Client, Clientset with GetKubeConfigAndClientWithConn() - PASS")
	printDiv()
	return nil
}

// testGetKubeConfigAndClientAdmin tests GetKubeConfigAndClient() with admin elevation
func testGetKubeConfigAndClientAdmin(clusterID, reason string) error {
	fmt.Printf("Testing backplane-admin client, clientset GetKubeConfigAndClient() for cluster:'%s'\n", clusterID)
	kubeCli, _, kubeClientSet, err := common.GetKubeConfigAndClient(clusterID, reason)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(kubeCli); err != nil {
		return err
	}

	openshiftMonitoringNamespace := "openshift-monitoring"
	podList, err := kubeClientSet.CoreV1().Pods(openshiftMonitoringNamespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("ClientSet list 'openshift-monitoring' pods failed, err:'%v'\n", err)
		return err
	}

	fmt.Printf("Got %d pods in namespace:'%s' :\n", len(podList.Items), openshiftMonitoringNamespace)
	for i, pod := range podList.Items {
		fmt.Printf("Got pod (%d/%d): '%s/%s' \n", i, len(podList.Items), pod.Namespace, pod.Name)
	}

	fmt.Println("bpadmin Create and test Kube Client, Clientset with GetKubeConfigAndClient() - PASS")
	printDiv()
	return nil
}

// testGetKubeConfigAndClientWithConnAdmin tests GetKubeConfigAndClientWithConn() with admin elevation
func testGetKubeConfigAndClientWithConnAdmin(clusterID string, ocmClient *sdk.Connection, reason string) error {
	fmt.Printf("Testing backplane-admin GetKubeConfigAndClientWithConn() for cluster:'%s'\n", clusterID)
	kubeCli, _, kubeClientSet, err := common.GetKubeConfigAndClientWithConn(clusterID, ocmClient, reason)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(kubeCli); err != nil {
		return err
	}

	podList, err := kubeClientSet.CoreV1().Pods("openshift-monitoring").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Printf("ClientSet list 'openshift-monitoring' pods failed, err:'%v'\n", err)
		return err
	}

	fmt.Printf("Got %d pods\n", len(podList.Items))
	for i, pod := range podList.Items {
		fmt.Printf("Got pod (%d/%d): '%s/%s' \n", i, len(podList.Items), pod.Namespace, pod.Name)
	}

	fmt.Println("bpadmin Create and test Kube Client, Clientset with GetKubeConfigAndClientWithConn() - PASS")
	printDiv()
	return nil
}

// testGetHiveBPWithoutElevation tests GetHiveBPClientForCluster() without elevation
func testGetHiveBPWithoutElevation(clusterID, hiveOcmURL string) error {
	fmt.Printf("Testing GetHiveBPClientForCluster() hive backplane connection w/o elevation\n")
	hiveBP, err := utils.GetHiveBPClientForCluster(clusterID, client.Options{}, "", hiveOcmURL)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(hiveBP); err != nil {
		return err
	}

	fmt.Println("Create and test GetHiveBPClientForCluster() without elevation reason - PASS")
	printDiv()
	return nil
}

// testGetHiveBPWithElevation tests GetHiveBPClientForCluster() with elevation
func testGetHiveBPWithElevation(clusterID, reason, hiveOcmURL string) error {
	fmt.Printf("Testing GetHiveBPClientForCluster() hive backplane connection with elevation\n")
	hiveBP, err := utils.GetHiveBPClientForCluster(clusterID, client.Options{}, reason, hiveOcmURL)
	if err != nil {
		return err
	}

	if err := dumpClusterOperators(hiveBP); err != nil {
		return err
	}

	clusterDep, err := getClusterDeployment(hiveBP, clusterID)
	if err != nil {
		return err
	}

	fmt.Printf("Fetched ClusterDeployment:'%s/%s' for cluster:'%s' from HIVE using elevated client\n", clusterDep.Namespace, clusterDep.Name, clusterID)
	fmt.Println("Create and test GetHiveBPClientForCluster() with elevation reason - PASS")
	printDiv()
	return nil
}

func (o *testHiveLoginOptions) run() error {
	// Initialize Hive OCM URL from args or config
	if len(o.hiveOcmURL) > 0 {
		fmt.Printf("Using Hive OCM URL set in args:'%s'\n", o.hiveOcmURL)
	} else {
		o.hiveOcmURL = viper.GetString("hive_ocm_url")
		if len(o.hiveOcmURL) > 0 {
			fmt.Printf("Got Hive OCM URL from viper vars:'%s'\n", o.hiveOcmURL)
		} else {
			fmt.Printf("No 'separate' Hive OCM URL set, using defaults set for target cluster.\n")
		}
	}

	o.reason = "Testing osdctl clients with cluster admin"

	// Setup: Create OCM connection and fetch target cluster
	ocmClient, cluster, clusterID, err := setupOCMConnection(o.clusterID)
	if err != nil {
		return err
	}
	defer ocmClient.Close()
	o.clusterID = clusterID

	// Setup: Build Hive OCM config if provided
	hiveOCMCfg, err := setupHiveOCMConfig(o.hiveOcmConfigPath, o.hiveOcmURL)
	if err != nil {
		return err
	}

	// Setup: Create Hive OCM connection if config was built
	hiveOCM, err := setupHiveOCMConnection(hiveOCMCfg, o.hiveOcmConfigPath)
	if err != nil {
		return err
	}

	// Setup: Fetch Hive cluster
	hiveCluster, err := setupHiveCluster(clusterID, ocmClient, hiveOCM)
	if err != nil {
		return err
	}

	// Run individual tests
	if err := testK8sNew(clusterID, cluster); err != nil {
		return err
	}

	if err := testK8sNewWithConn(hiveCluster, hiveOCM); err != nil {
		return err
	}

	if err := testK8sNewAsBackplaneClusterAdmin(hiveCluster, hiveOCM, clusterID, o.reason); err != nil {
		return err
	}

	if err := testGetKubeConfigAndClient(clusterID); err != nil {
		return err
	}

	if err := testGetKubeConfigAndClientWithConn(clusterID, ocmClient); err != nil {
		return err
	}

	if err := testGetKubeConfigAndClientAdmin(clusterID, o.reason); err != nil {
		return err
	}

	if err := testGetKubeConfigAndClientWithConnAdmin(clusterID, ocmClient, o.reason); err != nil {
		return err
	}

	if err := testGetHiveBPWithoutElevation(clusterID, o.hiveOcmURL); err != nil {
		return err
	}

	if err := testGetHiveBPWithElevation(clusterID, "Testing hive client backplane connections", o.hiveOcmURL); err != nil {
		return err
	}

	fmt.Println("All tests Passed")
	return nil
}
