package health

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	hypershift "github.com/openshift/hypershift/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// healthOptions defines the struct for running health command
// This command requires the ocm API Token https://cloud.redhat.com/openshift/token be available in the OCM_TOKEN env variable.

// newCmdHealth implements the health command to describe number of running instances in cluster and the expected number of nodes
func newCmdClusterHealth() *cobra.Command {
	h := &Health{}

	healthCmd := &cobra.Command{
		Use:               "health",
		Short:             "\n Describes health of cluster nodes and provides other cluster vitals. For hypershift clusters, requires previous login to the management cluster api server via `ocm login` and being tunneled to the backplane. \n \n Example: \" osdctl cluster health -C 12345678910 -p rhcontrol \" ",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return h.RunClusterHealth(context.Background())
		},
	}

	healthCmd.Flags().BoolVarP(&h.verbose, "verbose", "", false, "Verbose output")
	healthCmd.Flags().StringVarP(&h.clusterId, "cluster-id", "C", "", "Cluster ID")
	healthCmd.Flags().StringVarP(&h.awsProfile, "profile", "p", "", "AWS Profile")
	healthCmd.Flags().StringVarP(&h.environment, "env", "e", "production", "environment")
	healthCmd.MarkFlagRequired("cluster-id")
	healthCmd.MarkFlagRequired("profile")

	return healthCmd
}

func (h *Health) complete(cmd *cobra.Command, _ []string) error {

	return nil
}

type ClusterHealthCondensedObject struct {
	ID         string   `yaml:"ID"`
	Name       string   `yaml:"Name"`
	Provider   string   `yaml:"Provider"`
	Hypershift bool     `yaml:"Hypershift"`
	AZs        []string `yaml:"AZs"`
	Expected   struct {
		Master int         `yaml:"Master"`
		Infra  int         `yaml:"Infra"`
		Worker interface{} `yaml:"Worker"`
	} `yaml:"Expected nodes"`
	Actual struct {
		Total          int `yaml:"Total"`
		Stopped        int `yaml:"Stopped"`
		RunningMasters int `yaml:"Running Masters"`
		RunningInfra   int `yaml:"Running Infra"`
		RunningWorker  int `yaml:"Running Worker"`
	} `yaml:"Actual nodes"`
}

type ClusterHealthHypershiftObject struct {
	ID                string   `yaml:"ID"`
	Name              string   `yaml:"Name"`
	Provider          string   `yaml:"Provider"`
	Hypershift        bool     `yaml:"Hypershift"`
	ManagementCluster string   `yaml:"Management Cluster"`
	AZs               []string `yaml:"AZs"`
	Expected          struct {
		Worker interface{} `yaml:"Worker"`
	} `yaml:"Expected nodes"`
}

func (h *Health) RunClusterHealth(ctx context.Context) error {
	if err := h.New(); err != nil {
		return fmt.Errorf("failed to initialize command: %v", err)
	}

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	cluster := h.cluster
	if !cluster.Hypershift().Enabled() {
		healthObject := createHealthObject(cluster)

		if cluster.Nodes().AutoscaleCompute().MinReplicas() != 0 {
			min := strconv.Itoa(cluster.Nodes().AutoscaleCompute().MinReplicas())
			max := strconv.Itoa(cluster.Nodes().AutoscaleCompute().MaxReplicas())
			healthObject.Expected.Worker = string(fmt.Sprintf("%v - %v", min, max))
		}
		if cluster.Nodes().Compute() != 0 {
			healthObject.Expected.Worker = int(cluster.Nodes().Compute())
		}

		runningMasters := 0
		runningInfra := 0
		runningWorkers := 0
		totalStopped := 0
		totalCluster := 0

		var clusterHealthClient osdCloud.ClusterHealthClient
		var ownedLabel string
		infraID := cluster.InfraID()
		if cluster.CloudProvider().ID() == "gcp" {
			clusterHealthClient, err = osdCloud.NewGcpCluster(ocmClient, h.clusterId)
			if err != nil {
				return err
			}
			ownedLabel = "kubernetes-io-cluster-" + infraID
			defer clusterHealthClient.Close()
		} else if cluster.CloudProvider().ID() == "aws" {
			clusterHealthClient, err = osdCloud.NewAwsCluster(ocmClient, h.clusterId, h.awsProfile)
			if err != nil {
				return err
			}
			ownedLabel = "kubernetes.io/cluster/" + infraID
			defer clusterHealthClient.Close()
		} else {
			return errors.New(fmt.Sprintf("Unknown cloud provider found: %s", cluster.CloudProvider().ID()))
		}
		err = clusterHealthClient.Login()
		if err != nil {
			return err
		}
		for _, zone := range clusterHealthClient.GetAZs() {
			instances, err := clusterHealthClient.GetAllVirtualMachines(zone)
			if err != nil {
				return err
			}
			for _, instance := range instances {
				name := instance.Name
				state := instance.State
				labels := instance.Labels
				belongsToCluster := false
				for label := range labels {
					if label == ownedLabel {
						belongsToCluster = true
					}
				}
				if !belongsToCluster {
					if h.verbose {
						log.Printf("Skipping a machine not belonging to the cluster: %s\n", name)
					}
					continue
				}
				totalCluster += 1
				if state != "running" {
					totalStopped += 1
				} else {
					if strings.HasPrefix(name, infraID) && strings.Contains(name, "master") {
						runningMasters += 1
					} else if strings.HasPrefix(name, infraID) && strings.Contains(name, "infra") {
						runningInfra += 1
					} else if strings.HasPrefix(name, infraID) && strings.Contains(name, "worker") {
						runningWorkers += 1
					}
				}
			}
		}

		healthObject.Actual.Stopped = totalStopped
		healthObject.Actual.RunningMasters = runningMasters
		healthObject.Actual.RunningInfra = runningInfra
		healthObject.Actual.RunningWorker = runningWorkers
		healthObject.Actual.Total = totalCluster

		if err != nil {
			log.Fatalf("Error getting instances %v", err)
			return err
		}

		healthOutput, err := yaml.Marshal(&healthObject)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		fmt.Printf("\n \n")
		fmt.Println(string(healthOutput))
	} else {

		hsHealthObject := createHypershiftHealthObject(cluster)

		if cluster.Nodes().Compute() != 0 {
			hsHealthObject.Expected.Worker = int(cluster.Nodes().Compute())
		}
		mc := ""
		if cluster.Hypershift().Enabled() {
			hypershiftResp, err := ocmClient.ClustersMgmt().V1().Clusters().
				Cluster(cluster.ID()).
				Hypershift().
				Get().
				Send()
			if err != nil {
				return errors.New(fmt.Sprintf("Could not get hypershift response.  %s", err))
			}
			mc = hypershiftResp.Body().ManagementCluster()
		}

		nodepool, err := getNodepools(ctx)
		if err != nil {
			return err
		}

		hsHealthObject.ManagementCluster = mc

		hsHealthOutput, err := yaml.Marshal(&hsHealthObject)
		if err != nil {
			log.Fatalf("error: %v", err)
		}
		fmt.Printf("\n \n")
		fmt.Println(string(hsHealthOutput))
		fmt.Printf("\n")
		fmt.Println("oc get nodepool -n ocm-production-", h.clusterId)
		fmt.Printf("\n ")
		fmt.Println(nodepool)
		fmt.Printf("\n \n")

	}
	return nil
}

func createHealthObject(cluster *v1.Cluster) *ClusterHealthCondensedObject {

	var healthObject ClusterHealthCondensedObject

	cloudProvider := cluster.CloudProvider().ID()
	cloudProviderMessage := strings.ToUpper(cloudProvider)

	healthObject.ID = cluster.ID()
	healthObject.Name = cluster.Name()
	healthObject.Provider = cloudProviderMessage
	healthObject.AZs = cluster.Nodes().AvailabilityZones()
	healthObject.Hypershift = cluster.Hypershift().Enabled()
	healthObject.Expected.Infra = cluster.Nodes().Infra()
	healthObject.Expected.Master = cluster.Nodes().Master()

	return &healthObject
}

func createHypershiftHealthObject(cluster *v1.Cluster) *ClusterHealthHypershiftObject {

	var hsHealthObject ClusterHealthHypershiftObject

	cloudProvider := cluster.CloudProvider().ID()
	cloudProviderMessage := strings.ToUpper(cloudProvider)

	hsHealthObject.Hypershift = cluster.Hypershift().Enabled()
	hsHealthObject.ID = cluster.ID()
	hsHealthObject.Name = cluster.Name()
	hsHealthObject.Provider = cloudProviderMessage
	hsHealthObject.AZs = cluster.Nodes().AvailabilityZones()

	return &hsHealthObject
}
func getNodepools(ctx context.Context) (*hypershift.NodePool, error) {

	h := &Health{}
	if err := h.New(); err != nil {
		return nil, fmt.Errorf("failed to initialize command: %v", err)
	}

	namespace := "ocm-production-" + h.clusterId

	metav1.Kind(namespace)

	//("oc get nodepool -n ocm-production-%v", clusterID)

	np := &hypershift.NodePool{}

	err := h.hypershift.Get(ctx, client.ObjectKey{Namespace: namespace}, np)
	if err != nil {
		return nil, err
	}

	return np, nil

}
