package jumphost

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/spf13/cobra"
)

func newCmdDeleteJumphost() *cobra.Command {
	var (
		clusterId string
		subnetId  string
	)

	create := &cobra.Command{
		Use:          "delete",
		SilenceUsage: true,
		Short:        "Delete a jumphost created by `osdctl jumphost create`",
		Long: `Delete a jumphost created by "osdctl jumphost create"

  This command cleans up AWS resources created by "osdctl jumphost create" if it
  fails the customer should be notified as there will be leftover AWS resources
  in their account. This command is idempotent and safe to run over and over.

  Requires these permissions:
  {
    "Version": "2012-10-17",
    "Statement": [
      {
        "Action": [
          "ec2:AuthorizeSecurityGroupIngress",
          "ec2:CreateKeyPair",
          "ec2:CreateSecurityGroup",
          "ec2:CreateTags",
          "ec2:DeleteKeyPair",
          "ec2:DeleteSecurityGroup",
          "ec2:DescribeImages",
          "ec2:DescribeInstances",
          "ec2:DescribeKeyPairs",
          "ec2:DescribeSecurityGroups",
          "ec2:DescribeSubnets",
          "ec2:RunInstances",
          "ec2:TerminateInstances"
        ],
        "Effect": "Allow",
        "Resource": "*"
      }
    ]
  }`,
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

			return j.runDelete(context.TODO())
		},
	}

	create.Flags().StringVar(&subnetId, "subnet-id", "", "subnet id to search for and delete a jumphost in")
	create.MarkFlagRequired("subnet-id")

	return create
}

func (j *jumphostConfig) runDelete(ctx context.Context) error {
	if err := j.deleteEc2Jumphost(ctx); err != nil {
		return err
	}

	if err := j.deleteKeyPair(ctx); err != nil {
		return err
	}

	if err := j.deleteSecurityGroup(ctx); err != nil {
		return err
	}

	return nil
}

// deleteKeyPair searches for a EC2 key pairs by the expected tag filter and deletes the first matching key pair
func (j *jumphostConfig) deleteKeyPair(ctx context.Context) error {
	resp, err := j.awsClient.DescribeKeyPairs(ctx, &ec2.DescribeKeyPairsInput{
		Filters: generateTagFilters(j.tags),
	})
	if err != nil {
		return fmt.Errorf("failed to describe key pair: %w", err)
	}

	if len(resp.KeyPairs) == 0 {
		log.Println("no key pairs found to delete")
		return nil
	}

	log.Printf("deleting key pair: %s (%s)", *resp.KeyPairs[0].KeyName, *resp.KeyPairs[0].KeyPairId)
	_, err = j.awsClient.DeleteKeyPair(ctx, &ec2.DeleteKeyPairInput{
		KeyPairId: resp.KeyPairs[0].KeyPairId,
	})
	if err != nil {
		return fmt.Errorf("failed to delete keypair: %w", err)
	}

	return nil
}

// deleteSecurityGroup searches for security groups by the expected tag filter within the provided subnet's VPC and
// deletes the first matching security group
func (j *jumphostConfig) deleteSecurityGroup(ctx context.Context) error {
	vpcId, err := j.findVpcId(ctx)
	if err != nil {
		return err
	}

	resp, err := j.awsClient.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		Filters: append(generateTagFilters(j.tags), []types.Filter{
			{
				Name:   aws.String("group-name"),
				Values: []string{awsResourceName},
			},
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcId},
			},
		}...),
	})
	if err != nil {
		return fmt.Errorf("failed to describe security groups: %w", err)
	}

	if len(resp.SecurityGroups) == 0 {
		log.Println("no security groups found to delete")
		return nil
	}

	log.Printf("deleting security group: %s (%s)", *resp.SecurityGroups[0].GroupName, *resp.SecurityGroups[0].GroupId)
	_, err = j.awsClient.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{
		GroupId: resp.SecurityGroups[0].GroupId,
	})
	if err != nil {
		return fmt.Errorf("failed to delete security group: %w", err)
	}

	return nil
}

// deleteEc2Jumphost searches for EC2 instances by the expected tag filter within the provided subnet's VPC and
// terminates the first matching EC2 instance
func (j *jumphostConfig) deleteEc2Jumphost(ctx context.Context) error {
	vpcId, err := j.findVpcId(ctx)
	if err != nil {
		return err
	}

	describeInstancesResp, err := j.awsClient.DescribeInstances(ctx, &ec2.DescribeInstancesInput{
		Filters: append(generateTagFilters(j.tags), []types.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []string{vpcId},
			},
		}...),
	})
	if err != nil {
		return err
	}

	if len(describeInstancesResp.Reservations) == 0 || len(describeInstancesResp.Reservations[0].Instances) == 0 {
		log.Println("no EC2 instances found to terminate")
		return nil
	}

	log.Printf("terminating EC2 instance: %s", *describeInstancesResp.Reservations[0].Instances[0].InstanceId)
	_, err = j.awsClient.TerminateInstances(ctx, &ec2.TerminateInstancesInput{
		InstanceIds: []string{*describeInstancesResp.Reservations[0].Instances[0].InstanceId},
	})
	if err != nil {
		return err
	}

	log.Println("waiting for the EC2 instance to be in a terminated state")
	waiter := ec2.NewInstanceTerminatedWaiter(j.awsClient)
	if err := waiter.Wait(ctx, &ec2.DescribeInstancesInput{InstanceIds: []string{*describeInstancesResp.Reservations[0].Instances[0].InstanceId}}, 5*time.Minute); err != nil {
		return err
	}

	return nil
}
