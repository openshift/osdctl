/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cloudtrail

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/openshift-online/ocm-sdk-go/logging"

	"github.com/openshift/osdctl/pkg/osdCloud"
	"github.com/spf13/cobra"
)

// HelloCmd represents the Hello command
var whoamiCmd = &cobra.Command{
	Use:   "whoami",
	Short: "Prints out Hello Cloundtrail to the console",
	Long:  ``,
}

type requiredID struct {
	clusterID string
	log       logging.Logger
}

func whoami() error {

	cmd := fmt.Sprintf("whoami %s")
	output, err := exec.Command("bash", "-c", cmd).CombinedOutput()

	if err != nil {
		fmt.Printf("Failed: %s", strings.TrimSpace(string(output)))
		return err
	}
	return nil
}

func (o *requiredID) run() error {

	cfg, err := osdCloud.CreateAWSV2Config(o.clusterID)

	if err != nil {
		fmt.Errorf("Failed to get credentials", err)
		return err
	}
	awsClient := ec2.NewFromConfig(cfg)
	fmt.Println("[+] Getting Credentials")
	resp := whoami()

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
