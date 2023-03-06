package jumphost

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/spf13/cobra"
)

func newCmdCreateJumphost() *cobra.Command {
	var (
		clusterId string
		subnetId  string
	)

	create := &cobra.Command{
		Use:          "create",
		SilenceUsage: true,
		Short:        "Create a jumphost for emergency SSH access to a cluster's VMs",
		Long: `Create a jumphost for emergency SSH access to a cluster's VMs'

  NOTE: Only support key pairs currently

  This command automates the process of creating a jumphost in order to gain SSH access to a cluster's EC2 instances and
  should generally only be used as a last resort when the cluster's API server is otherwise inaccessible. It requires
  valid AWS credentials to be already set and a subnet ID in the associated AWS account. The provided subnet ID must
  be a public subnet.

  When the cluster's API server is accessible, prefer "oc debug node"`,
		Example: `
  # Create and delete a jumphost
  osdctl jumphost create --subnet-id public-subnet-id
  osdctl jumphost delete --subnet-id public-subnet-id`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			j, err := initJumphostConfig(context.TODO(), clusterId, subnetId)
			if err != nil {
				return err
			}

			return j.runCreate(context.TODO())
		},
	}

	// create.Flags().StringVarP(&clusterId, "cluster-id", "c", "", "OCM internal/external cluster id trying to access via a jumphost")
	create.Flags().StringVar(&subnetId, "subnet-id", "", "public subnet id to create a jumphost in")
	create.MarkFlagRequired("subnet-id")

	return create
}

func (j *jumphostConfig) runCreate(ctx context.Context) error {
	if err := j.createKeyPair(ctx); err != nil {
		return err
	}

	securityGroupId, err := j.createSecurityGroup(ctx)
	if err != nil {
		return err
	}

	if err := j.createEc2Jumphost(ctx, securityGroupId); err != nil {
		return err
	}

	log.Println(j.assembleNextSteps())
	return nil
}

// createKeyPair creates an EC2 key pair and attempts to save the private key to a temporary file and assign
// 400 permissions. If it is able to do so successfully, it stores the filepath in j.keyFilePath.
func (j *jumphostConfig) createKeyPair(ctx context.Context) error {
	resp, err := j.awsClient.CreateKeyPair(ctx, &ec2.CreateKeyPairInput{
		// TODO: Make name configurable if multiple SREs want to have an active jumphost
		KeyName:   aws.String(awsResourceName),
		KeyFormat: types.KeyFormatPem,
		KeyType:   types.KeyTypeEd25519,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeKeyPair,
				Tags:         j.tags,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create keypair: %w", err)
	}

	f, err := os.CreateTemp("", "jumphost_*.pem")
	if err != nil {
		log.Printf("failed to create temp file: %s, printing private key instead", err)
		log.Printf("created %s (%s). Save this private key\n%s", *resp.KeyName, *resp.KeyPairId, *resp.KeyMaterial)
		return nil
	}

	defer f.Close()
	if _, err := f.WriteString(*resp.KeyMaterial); err != nil {
		log.Printf("failed to create temp file: %s, printing private key instead", err)
		log.Printf("created %s (%s). Save this private key\n%s", *resp.KeyName, *resp.KeyPairId, *resp.KeyMaterial)
		return nil
	}

	if err := f.Chmod(0400); err != nil {
		log.Printf("failed to create temp file: %s, printing private key instead", err)
		log.Printf("created %s (%s). Save this private key\n%s", *resp.KeyName, *resp.KeyPairId, *resp.KeyMaterial)
		return nil
	}

	log.Printf("created file %s", f.Name())
	j.keyFilepath = f.Name()
	return nil
}

// createSecurityGroup creates a security group and creates a single inbound rule to allow the user's public IP to SSH.
func (j *jumphostConfig) createSecurityGroup(ctx context.Context) (string, error) {
	vpcId, err := j.findVpcId(ctx)
	if err != nil {
		return "", err
	}

	resp, err := j.awsClient.CreateSecurityGroup(ctx, &ec2.CreateSecurityGroupInput{
		Description: aws.String(awsResourceName),
		GroupName:   aws.String(awsResourceName),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeSecurityGroup,
				Tags:         j.tags,
			},
		},
		VpcId: aws.String(vpcId),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create security group: %w", err)
	}

	// Wait up to 1 minutes for the security group to exist
	waiter := ec2.NewSecurityGroupExistsWaiter(j.awsClient)
	if err := waiter.Wait(ctx, &ec2.DescribeSecurityGroupsInput{GroupIds: []string{*resp.GroupId}}, 1*time.Minute); err != nil {
		return "", fmt.Errorf("timed out waiting for security group to exist: %s", *resp.GroupId)
	}
	log.Printf("created security group: %s", *resp.GroupId)

	if err := j.allowJumphostSshFromIp(ctx, *resp.GroupId); err != nil {
		return *resp.GroupId, fmt.Errorf("failed to allow SSH to jumphost: %w", err)
	}

	return *resp.GroupId, nil
}

// createEc2Jumphost creates a t3.micro EC2 instance given a specific security group id
func (j *jumphostConfig) createEc2Jumphost(ctx context.Context, securityGroupId string) error {
	if j.subnetId == "" {
		return errors.New("could not create jumphost; subnet id must not be empty")
	}

	ami, err := j.findLatestJumphostAMI(ctx)
	if err != nil {
		return err
	}

	resp, err := j.awsClient.RunInstances(ctx, &ec2.RunInstancesInput{
		MaxCount: aws.Int32(1),
		MinCount: aws.Int32(1),
		BlockDeviceMappings: []types.BlockDeviceMapping{
			{
				DeviceName: aws.String("/dev/xvda"),
				Ebs: &types.EbsBlockDevice{
					DeleteOnTermination: aws.Bool(true),
					Encrypted:           aws.Bool(true),
				},
			},
		},
		ImageId:                           aws.String(ami),
		InstanceInitiatedShutdownBehavior: types.ShutdownBehaviorTerminate,
		InstanceType:                      types.InstanceTypeT3Micro,
		KeyName:                           aws.String(awsResourceName),
		NetworkInterfaces: []types.InstanceNetworkInterfaceSpecification{
			{
				AssociatePublicIpAddress: aws.Bool(true),
				DeleteOnTermination:      aws.Bool(true),
				DeviceIndex:              aws.Int32(0),
				Groups:                   []string{securityGroupId},
				SubnetId:                 aws.String(j.subnetId),
			},
		},
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInstance,
				Tags:         j.tags,
			},
		},
		// TODO: It would be nice to add user data which shut down the EC2 instance after 8 hours.
		// This would automatically terminate the EC2 instance in case we forget to clean up.
		UserData: nil,
	})
	if err != nil {
		return fmt.Errorf("failed to create jumphost EC2 instace: %w", err)
	}

	// Wait up to 5 minutes for the instance to be running
	// If it fails to come up in time, terminate it - we can always try again later
	log.Println("waiting for the EC2 instance to be in a running state")
	waiter := ec2.NewInstanceRunningWaiter(j.awsClient)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{*resp.Instances[0].InstanceId}}, 5*time.Minute); err != nil {
		if _, err := j.awsClient.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
			InstanceIds: []string{*resp.Instances[0].InstanceId},
		}); err != nil {
			return err
		}
		return fmt.Errorf("%s: terminated %s after timing out waiting for instance to be running", err, *resp.Instances[0].InstanceId)
	}

	describeInstancesResp, err := j.awsClient.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		InstanceIds: []string{*resp.Instances[0].InstanceId},
	})

	log.Printf("created EC2 jumphost: %s with public ip: %s", *describeInstancesResp.Reservations[0].Instances[0].InstanceId, *describeInstancesResp.Reservations[0].Instances[0].PublicIpAddress)
	j.ec2PublicIp = *describeInstancesResp.Reservations[0].Instances[0].PublicIpAddress
	return nil
}

// assembleNextSteps returns a string with helpful next steps for connecting to the created jumphost
func (j *jumphostConfig) assembleNextSteps() string {
	if j.ec2PublicIp == "" {
		return fmt.Sprintf("could not determine EC2 public ip - please verify, but something likely went wrong")
	}

	if j.keyFilepath != "" {
		return fmt.Sprintf("ssh -i %s ec2-user@%s", j.keyFilepath, j.ec2PublicIp)
	}

	return fmt.Sprintf("ssh-i ${private_key} ec2-user@%s", j.ec2PublicIp)
}

// findVpcId returns the AWS VPC ID of a provided jumphostConfig.
// Currently, requires that subnetId be defined.
func (j *jumphostConfig) findVpcId(ctx context.Context) (string, error) {
	if j.subnetId == "" {
		return "", errors.New("could not determine VPC; subnet id must not be empty")
	}

	log.Printf("searching for subnets by id: %s", j.subnetId)
	resp, err := j.awsClient.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{
		SubnetIds: []string{j.subnetId},
	})
	if err != nil {
		return "", err
	}

	if len(resp.Subnets) == 0 {
		return "", fmt.Errorf("found 0 subnets matching %s", j.subnetId)
	}

	return *resp.Subnets[0].VpcId, nil
}

// allowJumphostSshFromIp uses ec2:AuthorizeSecurityGroupIngress to create an inbound rule to allow
// TCP traffic on port 22 from the user's public IP.
func (j *jumphostConfig) allowJumphostSshFromIp(ctx context.Context, groupId string) error {
	ip, err := determinePublicIp()
	if err != nil {
		log.Printf("skipping modifying security group rule - failed to determine public ip: %s", err)
		return nil
	}

	if _, err := j.awsClient.AuthorizeSecurityGroupIngress(ctx, &ec2.AuthorizeSecurityGroupIngressInput{
		CidrIp:     aws.String(fmt.Sprintf("%s/32", ip)),
		FromPort:   aws.Int32(22),
		GroupId:    aws.String(groupId),
		IpProtocol: aws.String("tcp"),
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeSecurityGroupRule,
				Tags:         j.tags,
			},
		},
		ToPort: aws.Int32(22),
	}); err != nil {
		return err
	}
	log.Printf("authorized security group ingress for %s", ip)

	return nil
}

// determinePublicIp returns the public IP determined by a GET request to https://checkip.amazonaws.com
func determinePublicIp() (string, error) {
	resp, err := http.Get("https://checkip.amazonaws.com")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received error code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// The response has a trailing \n, so trim it off before validating the IP is valid
	ip := net.ParseIP(strings.TrimSpace(string(body)))
	if ip != nil {
		return ip.String(), nil
	}

	return "", fmt.Errorf("received an invalid ip: %s", ip)
}

// findLatestJumphostAMI finds the latest x86_64 Amazon Linux 2023 AMI
func (j *jumphostConfig) findLatestJumphostAMI(ctx context.Context) (string, error) {
	// https://docs.aws.amazon.com/AWSEC2/latest/APIReference/API_DescribeImages.html
	resp, err := j.awsClient.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Filters: []types.Filter{
			{
				Name:   aws.String("architecture"),
				Values: []string{string(types.ArchitectureTypeX8664)},
			},
			{
				Name:   aws.String("block-device-mapping.delete-on-termination"),
				Values: []string{"true"},
			},
			{
				Name:   aws.String("block-device-mapping.volume-type"),
				Values: []string{string(types.VolumeTypeGp3)},
			},
			{
				Name:   aws.String("creation-date"),
				Values: []string{"2023-*"},
			},
			{
				Name:   aws.String("description"),
				Values: []string{"Amazon Linux 2023 AMI*"},
			},
			{
				Name:   aws.String("image-type"),
				Values: []string{string(types.ImageTypeValuesMachine)},
			},
			{
				Name:   aws.String("is-public"),
				Values: []string{"true"},
			},
			{
				Name:   aws.String("root-device-type"),
				Values: []string{string(types.RootDeviceTypeEbs)},
			},
			{
				Name:   aws.String("state"),
				Values: []string{string(types.ImageStateAvailable)},
			},
		},
		ImageIds:          nil,
		IncludeDeprecated: aws.Bool(false),
		Owners:            []string{"amazon"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to describe images in order to launch an EC2 jumphost: %w", err)
	}

	var (
		latestTime time.Time
		latestAmi  string
	)

	for _, ami := range resp.Images {
		creationDate, err := time.Parse(time.RFC3339, *ami.CreationDate)
		if err != nil {
			continue
		}

		if creationDate.After(latestTime) {
			latestTime = creationDate
			latestAmi = *ami.ImageId
		}
	}

	log.Printf("found AMI: %s", latestAmi)
	return latestAmi, nil
}
