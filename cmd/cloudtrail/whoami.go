package cloudtrail

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws/arn"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"

	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/openshift/osdctl/pkg/utils"
	"github.com/spf13/cobra"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
)

var stsClient *sts.Client

type whoamiOptions struct {
	clusterID string
	cluster   *cmv1.Cluster
}

// whoamiCmd represents the whoami command
func newWhoamiCmd() *cobra.Command {
	ops := newwhoamiOptions()
	whoamiCmd := &cobra.Command{
		Use:   "whoami",
		Short: "Prints out User ARN to the console",
		Long:  ``,
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("")
			cmdutil.CheckErr(ops.complete(cmd, args))
			cmdutil.CheckErr(ops.run(cmd, args))

		},
	}
	whoamiCmd.Flags().StringVarP(&ops.clusterID, "cluster-id", "C", "", "Cluster ID")
	whoamiCmd.MarkFlagRequired("cluster-id")
	return whoamiCmd
}
func newwhoamiOptions() *whoamiOptions {
	return &whoamiOptions{}
}

func Whoami(stsClient sts.Client) (string, string, error) {

	ctx := context.TODO()
	callerIdentityOutput, err := stsClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", "", err
	}
	userArn, err := arn.Parse(*callerIdentityOutput.Arn)
	if err != nil {
		return "", "", err
	}

	return userArn.String(), userArn.AccountID, nil
}
func (o *whoamiOptions) complete(cmd *cobra.Command, _ []string) error {
	err := utils.IsValidClusterKey(o.clusterID)
	if err != nil {
		return err
	}

	connection, err := utils.CreateConnection()
	if err != nil {
		return err
	}

	defer connection.Close()

	cluster, err := utils.GetCluster(connection, o.clusterID)
	if err != nil {
		return err
	}

	o.cluster = cluster

	o.clusterID = cluster.ID()

	if strings.ToUpper(cluster.CloudProvider().ID()) != "AWS" {
		return errors.New("this command is only available for AWS clusters")
	}

	return nil
}

func (o *whoamiOptions) run(cmd *cobra.Command, _ []string) error {
	fmt.Println("[+] Trying to get credentials")
	ocmClient, err := utils.CreateConnection()
	if err != nil {
		return err
	}
	defer ocmClient.Close()

	if err != nil {
		return err
	}

	cfg, err := osdCloud.CreateAWSV2Config(ocmClient, o.cluster)
	if err != nil {
		fmt.Println("[-]Failed to get credentials....", err)
		fmt.Println("")
		return err
	}

	fmt.Println("[+] Getting Credentials")
	stsClient = sts.NewFromConfig(cfg)

	outputArn, outputID, err := Whoami(*stsClient)
	if err != nil {
		return err
	}
	fmt.Printf("[+] Your Current User ARN is:\n%s", outputArn)
	fmt.Printf("[+] Your Current User ID is:\n%s", outputID)
	fmt.Println("")
	return err

}
