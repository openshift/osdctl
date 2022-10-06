package network

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	ocmlog "github.com/openshift-online/ocm-sdk-go/logging"
	"github.com/openshift/osd-network-verifier/cmd/utils"
	"github.com/openshift/osd-network-verifier/pkg/proxy"
	"github.com/openshift/osd-network-verifier/pkg/verifier"
	"github.com/spf13/cobra"
)

var (
	defaultTags     = map[string]string{"osd-network-verifier": "owned", "red-hat-managed": "true", "Name": "osd-network-verifier"}
	regionEnvVarStr = "AWS_REGION"
	regionDefault   = "us-east-1"
)

type egressConfig struct {
	vpcSubnetID     string
	cloudImageID    string
	instanceType    string
	securityGroupId string
	cloudTags       map[string]string
	debug           bool
	region          string
	timeout         time.Duration
	kmsKeyID        string
	httpProxy       string
	httpsProxy      string
	CaCert          string
	noTls           bool
	awsProfile      string
}

func getDefaultRegion() string {
	val, present := os.LookupEnv(regionEnvVarStr)
	if present {
		return val
	}
	return regionDefault

}
func NewCmdValidateEgress() *cobra.Command {
	config := egressConfig{}

	validateEgressCmd := &cobra.Command{
		Use:   "verify-egress",
		Short: "Verify essential openshift domains are reachable from given subnet ID.",
		Long:  `Verify essential openshift domains are reachable from given subnet ID.`,
		Example: `For AWS, ensure your credential environment vars 
AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY (also AWS_SESSION_TOKEN for STS credentials) 
are set correctly before execution.

# Verify that essential openshift domains are reachable from a given SUBNET_ID
osdctl network verify-egress --subnet-id $(SUBNET_ID) --region $(AWS_REGION)`,
		Run: func(cmd *cobra.Command, args []string) {
			runEgressTest(config)
		},
	}

	validateEgressCmd.Flags().StringVar(&config.vpcSubnetID, "subnet-id", "", "source subnet ID")
	validateEgressCmd.Flags().StringVar(&config.cloudImageID, "image-id", "", "(optional) cloud image for the compute instance")
	validateEgressCmd.Flags().StringVar(&config.instanceType, "instance-type", "t3.micro", "(optional) compute instance type")
	validateEgressCmd.Flags().StringVar(&config.region, "region", getDefaultRegion(), fmt.Sprintf("(optional) compute instance region. If absent, environment var %[1]v will be used, if set", regionEnvVarStr, regionDefault))
	validateEgressCmd.Flags().StringToStringVar(&config.cloudTags, "cloud-tags", defaultTags, "(optional) comma-seperated list of tags to assign to cloud resources e.g. --cloud-tags key1=value1,key2=value2")
	validateEgressCmd.Flags().BoolVar(&config.debug, "debug", false, "(optional) if true, enable additional debug-level logging")
	validateEgressCmd.Flags().DurationVar(&config.timeout, "timeout", 1*time.Second, "(optional) timeout for individual egress verification requests")
	validateEgressCmd.Flags().StringVar(&config.kmsKeyID, "kms-key-id", "", "(optional) ID of KMS key used to encrypt root volumes of compute instances. Defaults to cloud account default key")
	validateEgressCmd.Flags().StringVarP(&config.awsProfile, "profile", "p", "", "(optional) AWS Profile")
	validateEgressCmd.Flags().StringVar(&config.securityGroupId, "security-group", "", "(optional) Security group to use for EC2 instance")
	validateEgressCmd.Flags().StringVar(&config.httpProxy, "http-proxy", "", "(optional) http-proxy to be used upon http requests being made by verifier, format: http://user:pass@x.x.x.x:8978")
	validateEgressCmd.Flags().StringVar(&config.httpsProxy, "https-proxy", "", "(optional) https-proxy to be used upon https requests being made by verifier, format: https://user:pass@x.x.x.x:8978")
	validateEgressCmd.Flags().StringVar(&config.CaCert, "cacert", "", "(optional) path to cacert file to be used upon https requests being made by verifier")
	validateEgressCmd.Flags().BoolVar(&config.noTls, "no-tls", false, "(optional) if true, ignore all ssl certificate validations on client-side.")

	if err := validateEgressCmd.MarkFlagRequired("subnet-id"); err != nil {
		validateEgressCmd.PrintErr(err)
		os.Exit(1)
	}

	return validateEgressCmd

}

func runEgressTest(config egressConfig) {

	// ctx
	ctx := context.TODO()

	// Create logger
	builder := ocmlog.NewStdLoggerBuilder()
	builder.Debug(config.debug)
	logger, err := builder.Build()
	if err != nil {
		fmt.Printf("Unable to build logger: %s\n", err.Error())
		os.Exit(1)
	}

	// Set Region
	if config.region == "" {
		config.region = "us-east-1"
	}

	// Set Up Proxy
	if config.CaCert != "" {
		// Read in the cert file
		cert, err := os.ReadFile(config.CaCert)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		// store string form of it
		// this was agreed with sda that they'll be communicating it as a string.
		config.CaCert = bytes.NewBuffer(cert).String()
	}

	p := proxy.ProxyConfig{
		HttpProxy:  config.httpProxy,
		HttpsProxy: config.httpsProxy,
		Cacert:     config.CaCert,
		NoTls:      config.noTls,
	}

	if len(config.cloudTags) == 0 {
		config.cloudTags = defaultTags
	}

	logger.Info(ctx, "Using region: %s", config.region)

	vei := verifier.ValidateEgressInput{
		Ctx:          context.TODO(),
		SubnetID:     config.vpcSubnetID,
		CloudImageID: config.cloudImageID,
		Timeout:      config.timeout,
		Tags:         config.cloudTags,
		InstanceType: config.instanceType,
		Proxy:        p,
	}

	vei.AWS = verifier.AwsEgressConfig{
		KmsKeyID:        config.kmsKeyID,
		SecurityGroupId: config.securityGroupId,
	}

	awsVerifier, err := utils.GetAwsVerifier(config.region, config.awsProfile, config.debug)
	if err != nil {
		fmt.Printf("could not build awsVerifier %v", err)
		os.Exit(1)
	}

	out := verifier.ValidateEgress(awsVerifier, vei)
	out.Summary(config.debug)

	if !out.IsSuccessful() {
		awsVerifier.Logger.Error(context.TODO(), "Failure!")
		os.Exit(1)
	}

	logger.Info(context.TODO(), "Success")
}
