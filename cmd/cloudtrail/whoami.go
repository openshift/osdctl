/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cloudtrail

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws/arn"

	"github.com/aws/aws-sdk-go-v2/service/sts"

	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/spf13/cobra"
)

// HelloCmd represents the Hello command
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Prints out Hello Cloundtrail to the console",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Hello Cloudtrail")
	},
}

type requiredID struct {
	clusterID string
}

func whoami(awsClient sts.Client) (string, error) {

	callerIdentityOutput, err := awsClient.GetCallerIdentity(&sts.GetCallerIdentityInput{})
	if err != nil {
		return "", err
	}
	userID, err := arn.Parse(*callerIdentityOutput.UserId)
	if err != nil {
		return "", err
	}

	return userID.AccountID, nil
}

func (o *requiredID) run() error {

	cfg, err := osdCloud.CreateAWSV2Config(o.clusterID)

	if err != nil {
		fmt.Errorf("Failed to get credentials", err)
		return err
	}
	fmt.Println("[+] Getting Credentials")
	awsClient := sts.NewFromConfig(cfg)
	if err != nil {
		return err
	}

	id, err = whoami(*awsClient)
	if err != nil {
		return err
	}
	fmt.Println()
	return err

}
func init() {

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// HelloCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// HelloCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
