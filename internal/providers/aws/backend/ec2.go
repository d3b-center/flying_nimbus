package aws

import (
	"context"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

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
	Volumes            []EbsVolume
	SecurityGroups     []SecurityGroup
}

type EbsVolume struct {
	VolumeID    string
	SizeGb        int32
	StorageType string
}

type ec2API interface {
	DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFuncs ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error)
	DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error)
}

type Ec2Service struct {
	api ec2API
}

func (i Ec2Instance) Title() string {
	return i.InstanceID
}

func (i Ec2Instance) Description() string {
	return i.Name
}

func (i Ec2Instance) FilterValue() string {
	return i.InstanceID
}

func InitEc2Service(cfg aws.Config) *Ec2Service {
	slog.Debug("Initializing EC2 Service")
	client := ec2.NewFromConfig(cfg)
	return &Ec2Service{
		api: client,
	}
}

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

func (e Ec2Service) ListInstances(ctx context.Context) ([]Ec2Instance, error) {
	var instances []Ec2Instance

	input := &ec2.DescribeInstancesInput{}

	for {
		result, err := e.api.DescribeInstances(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, reservation := range result.Reservations {
			for _, instance := range reservation.Instances {
				tagMap, name := extractTags(instance.Tags)
				securityGroups := extractSecurityGroups(instance.SecurityGroups)
				iamProfile := extractIamProfile(instance.IamInstanceProfile)
				launchTime := extractLaunchTime(instance.LaunchTime)
				volumes := e.getEbsVolumeData(ctx, instance.BlockDeviceMappings)


				instances = append(instances, Ec2Instance{
					InstanceID:         aws.ToString(instance.InstanceId),
					Name:               name,
					InstanceType:       string(instance.InstanceType),
					State:              string(instance.State.Name),
					PrivateIP:          aws.ToString(instance.PrivateIpAddress),
					PublicIP:           aws.ToString(instance.PublicIpAddress),
					VpcID:              aws.ToString(instance.VpcId),
					Tags:               tagMap,
					SubnetID:           aws.ToString(instance.SubnetId),
					IamInstanceProfile: iamProfile,
					LaunchTime:         launchTime,
					Volumes:            volumes,
					SecurityGroups:     securityGroups,
				})
			}
		}

		if result.NextToken == nil {
			break
		}

		input.NextToken = result.NextToken
	}

	return instances, nil
}

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

func extractSecurityGroups(sgs []types.GroupIdentifier) []SecurityGroup {
	var securityGroups []SecurityGroup
	for _, sg := range sgs {
		if sg.GroupId != nil {
			securityGroups = append(securityGroups, SecurityGroup{Id: *sg.GroupId})
		}
	}
	return securityGroups
}

func extractIamProfile(profile *types.IamInstanceProfile) string {
	if profile != nil && profile.Arn != nil {
		return *profile.Arn
	}
	return ""
}

func extractLaunchTime(launchTime *time.Time) string {
	if launchTime != nil {
		return launchTime.Format(time.RFC3339)
	}
	return ""
}