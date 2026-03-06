package aws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const (
	// Bastions are mostly named aws-infra-bastion-ssm-ec2-*, but sometimes Bastion
	bastionTag = "*astion*"
)

// Ec2Instance represents an EC2 instance with its metadata.
type Ec2Instance struct {
	InstanceID         string
	Name               string
	InstanceType       string
	AmiImage           string
	State              string
	PrivateIP          string
	PublicIP           string
	VpcID              string
	Tags               map[string]string
	SubnetID           string
	IamInstanceProfile string
	LaunchTime         string
	VolumeIds          []string
	SecurityGroupIds   []string
}

// EbsVolume represents an EBS volume attached to an instance.
type EbsVolume struct {
	VolumeID    string
	SizeGb      int32
	StorageType string
}

type ec2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFuncs ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
	StartInstances(ctx context.Context, params *ec2.StartInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StartInstancesOutput, error)
	StopInstances(ctx context.Context, params *ec2.StopInstancesInput, optFns ...func(*ec2.Options)) (*ec2.StopInstancesOutput, error)
}

// Ec2Service provides methods for interacting with EC2 instances.
type Ec2Service struct {
	api ec2API
}

// Title returns the instance ID for list display.
func (i Ec2Instance) Title() string {
	return i.InstanceID
}

// Description returns the instance name for list display.
func (i Ec2Instance) Description() string {
	return i.Name
}

// FilterValue returns the instance ID for list filtering.
func (i Ec2Instance) FilterValue() string {
	return i.InstanceID
}

// InitEc2Service creates a new EC2 service client.
func InitEc2Service(cfg aws.Config) *Ec2Service {
	slog.Debug("Initializing EC2 Service")
	client := ec2.NewFromConfig(cfg)
	return &Ec2Service{
		api: client,
	}
}

// GetVolumeDetails retrieves detailed information for the given volume IDs.
func (e Ec2Service) GetVolumeDetails(ctx context.Context, volumeIDs []string) ([]EbsVolume, error) {
	input := &ec2.DescribeVolumesInput{
		VolumeIds: volumeIDs,
	}

	result, err := e.api.DescribeVolumes(ctx, input)
	if err != nil {
		return nil, err
	}

	var volumes []EbsVolume
	for _, vol := range result.Volumes {
		volumes = append(volumes, EbsVolume{
			VolumeID:    aws.ToString(vol.VolumeId),
			SizeGb:      aws.ToInt32(vol.Size),
			StorageType: string(vol.VolumeType),
		})
	}

	return volumes, nil
}

// StartInstance attempts to start EC2 instance given InstanceId
func (e Ec2Service) StartInstance(ctx context.Context, instanceId string) error {
	input := ec2.StartInstancesInput{InstanceIds: []string{instanceId}}
	_, err := e.api.StartInstances(ctx, &input)

	return err
}

// StopInstance attempts to stop EC2 instance given InstanceId
func (e Ec2Service) StopInstance(ctx context.Context, instanceId string) error {
	input := ec2.StopInstancesInput{InstanceIds: []string{instanceId}}
	_, err := e.api.StopInstances(ctx, &input)

	return err
}

// Get all Bastion host instances
func (e Ec2Service) FindBastionHost(ctx context.Context, vpcId string) (Ec2Instance, error) {
	input := ec2.DescribeInstancesInput{}
	input.Filters = []types.Filter{
		{
			Name:   aws.String("vpc-id"),
			Values: []string{vpcId},
		},
		{
			Name:   aws.String("tag:Name"),
			Values: []string{bastionTag},
		},
		{
			Name:   aws.String("instance-state-name"),
			Values: []string{"running"},
		},
	}

	instances, err := e.ListInstancesWithFilter(ctx, input)
	if err != nil {
		return Ec2Instance{}, err
	}

	count := len(instances)
	if count != 1 {
		return Ec2Instance{}, fmt.Errorf("Wrong number of instances found: %d", count)
	}

	return instances[0], nil
}

// ListInstances with an input filter
func (e Ec2Service) ListInstancesWithFilter(ctx context.Context, input ec2.DescribeInstancesInput) ([]Ec2Instance, error) {
	return e.paginatedDescribeInstances(ctx, input)
}

// ListInstances retrieves all EC2 instances with pagination.
func (e Ec2Service) ListInstances(ctx context.Context) ([]Ec2Instance, error) {
	return e.paginatedDescribeInstances(ctx, ec2.DescribeInstancesInput{})
}

// processPage fetches and processes a single page of EC2 instances.
func (e Ec2Service) paginatedDescribeInstances(ctx context.Context, input ec2.DescribeInstancesInput) ([]Ec2Instance, error) {

	instances := make([]Ec2Instance, 0)

	for {
		ec2s, nextToken, err := e.fetchEc2Instances(ctx, input)
		if err != nil {
			return nil, err
		}

		instances = append(instances, ec2s...)
		input.NextToken = &nextToken

		if nextToken == "" {
			break
		}
	}
	return instances, nil
}

func (e Ec2Service) fetchEc2Instances(ctx context.Context, input ec2.DescribeInstancesInput) ([]Ec2Instance, string, error) {
	result, err := e.api.DescribeInstances(ctx, &input)
	if err != nil {
		return nil, "", err
	}

	instances := make([]Ec2Instance, 0, len(result.Reservations))
	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			tagMap, name := extractTags(instance.Tags)
			securityGroups := extractSecurityGroups(instance.SecurityGroups)
			iamProfile := extractIamProfile(instance.IamInstanceProfile)
			launchTime := extractLaunchTime(instance.LaunchTime)
			volumes := e.getEbsVolumeIds(instance.BlockDeviceMappings)
			instanceState := extractInstanceState(instance.State)

			instances = append(instances, Ec2Instance{
				InstanceID:         aws.ToString(instance.InstanceId),
				Name:               name,
				InstanceType:       string(instance.InstanceType),
				State:              instanceState,
				PrivateIP:          aws.ToString(instance.PrivateIpAddress),
				PublicIP:           aws.ToString(instance.PublicIpAddress),
				VpcID:              aws.ToString(instance.VpcId),
				Tags:               tagMap,
				SubnetID:           aws.ToString(instance.SubnetId),
				IamInstanceProfile: iamProfile,
				LaunchTime:         launchTime,
				VolumeIds:          volumes,
				SecurityGroupIds:   securityGroups,
			})
		}
	}
	return instances, aws.ToString(result.NextToken), nil
}

// getEbsVolumeData retrieves volume ids for the given block device mappings.
func (e Ec2Service) getEbsVolumeIds(bdms []types.InstanceBlockDeviceMapping) []string {
	var volumeIds []string
	for _, bdm := range bdms {
		if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil {
			volumeIds = append(volumeIds, aws.ToString(bdm.Ebs.VolumeId))
		}
	}
	return volumeIds
}

// getEbsVolumeData retrieves volume details for the given block device mappings.
func (e Ec2Service) getEbsVolumeData(ctx context.Context, bdms []types.InstanceBlockDeviceMapping) []EbsVolume {
	var volumeIds []string
	for _, bdm := range bdms {
		if bdm.Ebs != nil && bdm.Ebs.VolumeId != nil {
			volumeIds = append(volumeIds, aws.ToString(bdm.Ebs.VolumeId))
		}
	}

	var volumes []EbsVolume
	var err error
	if len(volumeIds) > 0 {
		volumes, err = e.GetVolumeDetails(ctx, volumeIds)
		if err != nil {
			slog.Warn("Failed to get volume details")
			volumes = []EbsVolume{}
		}
	}

	return volumes
}

// extractTags converts EC2 tags to a map and extracts the Name tag.
func extractTags(tags []types.Tag) (map[string]string, string) {
	tagMap := make(map[string]string)
	name := ""

	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagMap[*tag.Key] = *tag.Value
			if *tag.Key == "Name" {
				name = *tag.Value
			}
		}
	}

	return tagMap, name
}

// extractSecurityGroups extracts security group IDs from group identifiers.
func extractSecurityGroups(sgs []types.GroupIdentifier) []string {
	var securityGroupIds []string
	for _, sg := range sgs {
		if sg.GroupId != nil {
			securityGroupIds = append(securityGroupIds, aws.ToString(sg.GroupId))
		}
	}
	return securityGroupIds
}

// extractIamProfile extracts the IAM instance profile ARN.
func extractIamProfile(profile *types.IamInstanceProfile) string {
	if profile != nil && profile.Arn != nil {
		return *profile.Arn
	}
	return ""
}

// extractLaunchTime formats the instance launch time as RFC3339.
func extractLaunchTime(launchTime *time.Time) string {
	if launchTime != nil {
		return launchTime.Format(time.RFC3339)
	}
	return ""
}

// extractInstanceState extracts the instance state name.
func extractInstanceState(state *types.InstanceState) string {
	if state != nil {
		return string(state.Name)
	}
	return ""
}
