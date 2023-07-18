package cluster

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	v1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

// healthOptions defines the struct for running health command
// This command requires the ocm API Token https://cloud.redhat.com/openshift/token be available in the OCM_TOKEN env variable.

type healthOptions struct {
	clusterID  string
	output     string
	verbose    bool
	awsProfile string
}

// newCmdHealth implements the health command to describe number of running instances in cluster and the expected number of nodes
func newCmdHealth() *cobra.Command {
	ops := newHealthOptions()
	healthCmd := &cobra.Command{
		Use:               "health",
		Short:             "Describes health of cluster nodes and provides other cluster vitals.",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run())
		},
	}

	healthCmd.Flags().BoolVarP(&ops.verbose, "verbose", "", false, "Verbose output")
	healthCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	healthCmd.Flags().StringVarP(&ops.awsProfile, "profile", "p", "", "AWS Profile")
	healthCmd.MarkFlagRequired("cluster-id")
	return healthCmd
}

func newHealthOptions() *healthOptions {
	return &healthOptions{}
}

func (o *healthOptions) complete(cmd *cobra.Command, _ []string) error {

	return nil
}

type ClusterHealthCondensedObject struct {
	ID       string   `yaml:"ID"`
	Name     string   `yaml:"Name"`
	Provider string   `yaml:"Provider"`
	AZs      []string `yaml:"AZs"`
	Expected struct {
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

func (o *healthOptions) run() error {

	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	clusterResp, err := ocmClient.ClustersMgmt().V1().Clusters().Cluster(o.clusterID).Get().Send()
	if err != nil {
		fmt.Println(err)
		return err
	}
	cluster := clusterResp.Body()
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
		clusterHealthClient, err = osdCloud.NewGcpCluster(ocmClient, o.clusterID)
		if err != nil {
			return err
		}
		ownedLabel = "kubernetes-io-cluster-" + infraID
		defer clusterHealthClient.Close()
	} else if cluster.CloudProvider().ID() == "aws" {
		clusterHealthClient, err = osdCloud.NewAwsCluster(ocmClient, o.clusterID, o.awsProfile)
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
				if o.verbose {
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
	healthObject.Expected.Infra = cluster.Nodes().Infra()
	healthObject.Expected.Master = cluster.Nodes().Master()

	return &healthObject
}
