package aws

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type EC2Instance struct {
	InstanceID             string
	Name                   string
	InstanceType           string
	State                  string
	PrivateIP              string
	PublicIP               string
	VpcID                  string
	Tags                   map[string]string
	SubnetID               string
	IamInstanceProfile     string
	IamInstanceProfileName string
	LaunchTime             string
}

type EC2Service struct {
	client *ec2.Client
}

func NewEC2Service(ctx context.Context) (*EC2Service, error) {
	// Load AWS config (uses environment variables or ~/.aws/credentials)
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &EC2Service{
		client: ec2.NewFromConfig(cfg),
	}, nil
}

func (s *EC2Service) ListInstances(ctx context.Context) ([]EC2Instance, error) {
	// Describe all EC2 instances
	result, err := s.client.DescribeInstances(ctx, &ec2.DescribeInstancesInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	var instances []EC2Instance

	// Parse the response
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			ec2Instance := EC2Instance{
				InstanceID:   aws.ToString(instance.InstanceId),
				InstanceType: string(instance.InstanceType),
				State:        string(instance.State.Name),
				PrivateIP:    aws.ToString(instance.PrivateIpAddress),
				PublicIP:     aws.ToString(instance.PublicIpAddress),
				VpcID:        aws.ToString(instance.VpcId),
				SubnetID:     aws.ToString(instance.SubnetId),
				Tags:         make(map[string]string),
			}

			if instance.IamInstanceProfile != nil {
				ec2Instance.IamInstanceProfile = aws.ToString(instance.IamInstanceProfile.Arn)

				// Extract friendly name from ARN
				// ARN format: arn:aws:iam::123456789012:instance-profile/MyProfileName
				arn := ec2Instance.IamInstanceProfile
				parts := strings.Split(arn, "/")
				if len(parts) > 1 {
					ec2Instance.IamInstanceProfileName = parts[len(parts)-1]
				}
			}

			if instance.LaunchTime != nil {
				ec2Instance.LaunchTime = instance.LaunchTime.Format("2006-01-02 15:04:05")
			}

			// Extract tag
			for _, tag := range instance.Tags {
				key := aws.ToString(tag.Key)
				value := aws.ToString(tag.Value)
				ec2Instance.Tags[key] = value

				// Special handling for Name tag
				if key == "Name" {
					ec2Instance.Name = value
				}
			}

			// If no Name tag, use instance ID
			if ec2Instance.Name == "" {
				ec2Instance.Name = ec2Instance.InstanceID
			}

			instances = append(instances, ec2Instance)
		}
	}

	return instances, nil
}

func (s *EC2Service) StartInstances(ctx context.Context, instanceID string) (*ec2.StartInstancesOutput, error) {
	input := &ec2.StartInstancesInput{
		InstanceIds: []string{instanceID},
	}

	result, err := s.client.StartInstances(ctx, input)

	return result, err
}

func (s *EC2Service) StopInstance(ctx context.Context, instanceID string) (*ec2.StopInstancesOutput, error) {
	input := &ec2.StopInstancesInput{
		InstanceIds: []string{instanceID},
	}

	result, err := s.client.StopInstances(ctx, input)

	return result, err
}

func (s *EC2Service) TerminateInstance(ctx context.Context, instanceID string) (*ec2.TerminateInstancesOutput, error) {
	input := &ec2.TerminateInstancesInput{
		InstanceIds: []string{instanceID},
	}

	result, err := s.client.TerminateInstances(ctx, input)

	return result, err
}
