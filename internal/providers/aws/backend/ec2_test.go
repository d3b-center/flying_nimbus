package aws

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Mock EC2 API
type mockEc2API struct {
	mock.Mock
}

func (m *mockEc2API) DescribeInstances(ctx context.Context, params *ec2.DescribeInstancesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeInstancesOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ec2.DescribeInstancesOutput), args.Error(1)
}

func (m *mockEc2API) DescribeVolumes(ctx context.Context, params *ec2.DescribeVolumesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeVolumesOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ec2.DescribeVolumesOutput), args.Error(1)
}

func TestListInstances_Success(t *testing.T) {
	// Setup
	mockAPI := new(mockEc2API)
	service := Ec2Service{api: mockAPI}
	ctx := context.Background()

	launchTime := time.Now()
	
	// Mock response
	mockOutput := &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId:       aws.String("i-12345"),
						InstanceType:     types.InstanceTypeT3Medium,
						PrivateIpAddress: aws.String("10.0.1.100"),
						PublicIpAddress:  aws.String("54.1.2.3"),
						VpcId:            aws.String("vpc-123"),
						SubnetId:         aws.String("subnet-456"),
						LaunchTime:       &launchTime,
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						Tags: []types.Tag{
							{Key: aws.String("Name"), Value: aws.String("test-instance")},
							{Key: aws.String("Environment"), Value: aws.String("dev")},
						},
						SecurityGroups: []types.GroupIdentifier{
							{GroupId: aws.String("sg-123")},
						},
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
									VolumeID: aws.String("vol-123"),
								},
							},
						},
						IamInstanceProfile: &types.IamInstanceProfile{
							Arn: aws.String("arn:aws:iam::123:instance-profile/test"),
						},
					},
				},
			},
		},
		NextToken: nil,
	}

	mockVolOutput := &ec2.DescribeVolumesOutput{
		Volumes: []types.Volume{
			{
				VolumeId:   aws.String("vol-123"),
				Size:       aws.Int32(100),
				VolumeType: types.VolumeTypeGp3,
				Encrypted:  aws.Bool(true),
			},
		},
	}

	mockAPI.On("DescribeInstances", ctx, mock.Anything).Return(mockOutput, nil)
	mockAPI.On("DescribeVolumes", ctx, mock.Anything).Return(mockVolOutput, nil)

	// Execute
	instances, err := service.ListInstances(ctx)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, instances, 1)
	
	instance := instances[0]
	assert.Equal(t, "i-12345", instance.InstanceID)
	assert.Equal(t, "test-instance", instance.Name)
	assert.Equal(t, "t3.medium", instance.InstanceType)
	assert.Equal(t, "running", instance.State)
	assert.Equal(t, "10.0.1.100", instance.PrivateIP)
	assert.Equal(t, "54.1.2.3", instance.PublicIP)
	assert.Equal(t, "vpc-123", instance.VpcID)
	assert.Equal(t, "subnet-456", instance.SubnetID)
	assert.Equal(t, "arn:aws:iam::123:instance-profile/test", instance.IamInstanceProfile)
	
	// Check tags
	assert.Equal(t, "test-instance", instance.Tags["Name"])
	assert.Equal(t, "dev", instance.Tags["Environment"])
	
	// Check security groups
	assert.Len(t, instance.SecurityGroups, 1)
	assert.Equal(t, "sg-123", instance.SecurityGroups[0].Id)
	
	// Check volumes
	assert.Len(t, instance.Volumes, 1)
	assert.Equal(t, "vol-123", instance.Volumes[0].VolumeID)
	assert.Equal(t, "100", instance.Volumes[0].Size)
	assert.Equal(t, "gp3", instance.Volumes[0].StorageType)

	mockAPI.AssertExpectations(t)
}

func TestListInstances_WithPagination(t *testing.T) {
	// Setup
	mockAPI := new(mockEc2API)
	service := Ec2Service{api: mockAPI}
	ctx := context.Background()

	// First page
	firstOutput := &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId:   aws.String("i-first"),
						InstanceType: types.InstanceTypeT3Micro,
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
					},
				},
			},
		},
		NextToken: aws.String("token123"),
	}

	// Second page
	secondOutput := &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId:   aws.String("i-second"),
						InstanceType: types.InstanceTypeT3Small,
						State: &types.InstanceState{
							Name: types.InstanceStateNameStopped,
						},
					},
				},
			},
		},
		NextToken: nil,
	}

	mockAPI.On("DescribeInstances", ctx, mock.MatchedBy(func(input *ec2.DescribeInstancesInput) bool {
		return input.NextToken == nil
	})).Return(firstOutput, nil).Once()

	mockAPI.On("DescribeInstances", ctx, mock.MatchedBy(func(input *ec2.DescribeInstancesInput) bool {
		return input.NextToken != nil && *input.NextToken == "token123"
	})).Return(secondOutput, nil).Once()

	mockAPI.On("DescribeVolumes", ctx, mock.Anything).Return(&ec2.DescribeVolumesOutput{}, nil)

	// Execute
	instances, err := service.ListInstances(ctx)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, instances, 2)
	assert.Equal(t, "i-first", instances[0].InstanceID)
	assert.Equal(t, "i-second", instances[1].InstanceID)

	mockAPI.AssertExpectations(t)
}

func TestListInstances_EmptyResult(t *testing.T) {
	// Setup
	mockAPI := new(mockEc2API)
	service := Ec2Service{api: mockAPI}
	ctx := context.Background()

	mockOutput := &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{},
		NextToken:    nil,
	}

	mockAPI.On("DescribeInstances", ctx, mock.Anything).Return(mockOutput, nil)

	// Execute
	instances, err := service.ListInstances(ctx)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, instances, 0)

	mockAPI.AssertExpectations(t)
}

func TestGetVolumeDetails_Success(t *testing.T) {
	// Setup
	mockAPI := new(mockEc2API)
	service := Ec2Service{api: mockAPI}
	ctx := context.Background()

	volumeIDs := []string{"vol-123", "vol-456"}

	mockOutput := &ec2.DescribeVolumesOutput{
		Volumes: []types.Volume{
			{
				VolumeId:   aws.String("vol-123"),
				Size:       aws.Int32(50),
				VolumeType: types.VolumeTypeGp3,
				Encrypted:  aws.Bool(true),
			},
			{
				VolumeId:   aws.String("vol-456"),
				Size:       aws.Int32(100),
				VolumeType: types.VolumeTypeIo2,
				Encrypted:  aws.Bool(false),
			},
		},
	}

	mockAPI.On("DescribeVolumes", ctx, mock.Anything).Return(mockOutput, nil)

	// Execute
	volumes, err := service.GetVolumeDetails(ctx, volumeIDs)

	// Assert
	assert.NoError(t, err)
	assert.Len(t, volumes, 2)
	assert.Equal(t, "vol-123", volumes[0].VolumeID)
	assert.Equal(t, "50", volumes[0].Size)
	assert.Equal(t, "gp3", volumes[0].StorageType)
	assert.Equal(t, "vol-456", volumes[1].VolumeID)

	mockAPI.AssertExpectations(t)
}

func TestGetVolumeDetails_EmptyInput(t *testing.T) {
	// Setup
	mockAPI := new(mockEc2API)
	service := Ec2Service{api: mockAPI}
	ctx := context.Background()

	// Execute
	volumes, err := service.GetVolumeDetails(ctx, []string{})

	// Assert
	assert.NoError(t, err)
	assert.Len(t, volumes, 0)
	mockAPI.AssertNotCalled(t, "DescribeVolumes")
}