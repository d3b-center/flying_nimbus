package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockEc2API struct {
	instancesOutput *ec2.DescribeInstancesOutput
	volumesOutput   *ec2.DescribeVolumesOutput
	instancesErr    error
	volumesErr      error
}

func (m *mockEc2API) DescribeInstances(
	ctx context.Context,
	params *ec2.DescribeInstancesInput,
	opts ...func(*ec2.Options),
) (*ec2.DescribeInstancesOutput, error) {
	if m.instancesErr != nil {
		return nil, m.instancesErr
	}
	return m.instancesOutput, nil
}

func (m *mockEc2API) DescribeVolumes(
	ctx context.Context,
	params *ec2.DescribeVolumesInput,
	opts ...func(*ec2.Options),
) (*ec2.DescribeVolumesOutput, error) {
	if m.volumesErr != nil {
		return nil, m.volumesErr
	}
	return m.volumesOutput, nil
}

func (m *mockEc2API) StartInstances(
	ctx context.Context,
	params *ec2.StartInstancesInput,
	opts ...func(*ec2.Options),
) (*ec2.StartInstancesOutput, error) {
	return nil, nil
}

func (m *mockEc2API) StopInstances(
	ctx context.Context,
	params *ec2.StopInstancesInput,
	opts ...func(*ec2.Options),
) (*ec2.StopInstancesOutput, error) {
	return nil, nil
}

func TestEc2Instance_ListItemInterface(t *testing.T) {
	inst := Ec2Instance{
		InstanceID: "i-12345",
		Name:       "test-instance",
	}

	if inst.Title() != "i-12345" {
		t.Errorf("Title() = %q, want %q", inst.Title(), "i-12345")
	}

	if inst.Description() != "test-instance" {
		t.Errorf("Description() = %q, want %q", inst.Description(), "test-instance")
	}

	if inst.FilterValue() != "i-12345" {
		t.Errorf("FilterValue() = %q, want %q", inst.FilterValue(), "i-12345")
	}
}

func TestEc2Service_ListInstances_Success(t *testing.T) {
	launchTime := time.Now()

	mock := &mockEc2API{
		instancesOutput: &ec2.DescribeInstancesOutput{
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
								{GroupId: aws.String("sg-111")},
								{GroupId: aws.String("sg-222")},
							},
							BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
								{
									Ebs: &types.EbsInstanceBlockDevice{
										VolumeId: aws.String("vol-abc"),
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
		},
		volumesOutput: &ec2.DescribeVolumesOutput{
			Volumes: []types.Volume{
				{
					VolumeId:   aws.String("vol-abc"),
					Size:       aws.Int32(100),
					VolumeType: types.VolumeTypeGp3,
				},
			},
		},
	}

	svc := &Ec2Service{api: mock}

	instances, err := svc.ListInstances(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	inst := instances[0]

	if inst.InstanceID != "i-12345" {
		t.Errorf("InstanceID = %q", inst.InstanceID)
	}
	if inst.Name != "test-instance" {
		t.Errorf("Name = %q", inst.Name)
	}
	if inst.InstanceType != "t3.medium" {
		t.Errorf("InstanceType = %q", inst.InstanceType)
	}
	if inst.State != "running" {
		t.Errorf("State = %q", inst.State)
	}
	if inst.PrivateIP != "10.0.1.100" {
		t.Errorf("PrivateIP = %q", inst.PrivateIP)
	}
	if inst.VpcID != "vpc-123" {
		t.Errorf("VpcID = %q", inst.VpcID)
	}
	if len(inst.SecurityGroupIds) != 2 {
		t.Errorf("expected 2 SGs, got %d", len(inst.SecurityGroupIds))
	}
	if len(inst.Volumes) != 1 {
		t.Errorf("expected 1 volume, got %d", len(inst.Volumes))
	}
	if inst.Volumes[0].VolumeID != "vol-abc" {
		t.Errorf("VolumeID = %q", inst.Volumes[0].VolumeID)
	}
	if inst.Volumes[0].SizeGb != 100 {
		t.Errorf("Volume Size = %q", inst.Volumes[0].SizeGb)
	}
	if len(inst.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(inst.Tags))
	}
}

func TestEc2Service_ListInstances_Error(t *testing.T) {
	mock := &mockEc2API{
		instancesErr: errors.New("boom"),
	}

	svc := &Ec2Service{api: mock}

	instances, err := svc.ListInstances(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if instances != nil {
		t.Errorf("expected nil instances on error")
	}
}

func TestEc2Service_ListInstances_EmptyResult(t *testing.T) {
	mock := &mockEc2API{
		instancesOutput: &ec2.DescribeInstancesOutput{
			Reservations: []types.Reservation{},
		},
	}

	svc := &Ec2Service{api: mock}

	instances, err := svc.ListInstances(context.Background())

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}
}

func TestEc2Service_GetVolumeDetails_Success(t *testing.T) {
	mock := &mockEc2API{
		volumesOutput: &ec2.DescribeVolumesOutput{
			Volumes: []types.Volume{
				{
					VolumeId:   aws.String("vol-123"),
					Size:       aws.Int32(50),
					VolumeType: types.VolumeTypeGp3,
				},
				{
					VolumeId:   aws.String("vol-456"),
					Size:       aws.Int32(100),
					VolumeType: types.VolumeTypeIo2,
				},
			},
		},
	}

	svc := &Ec2Service{api: mock}

	volumes, err := svc.GetVolumeDetails(context.Background(), []string{"vol-123", "vol-456"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(volumes) != 2 {
		t.Fatalf("expected 2 volumes, got %d", len(volumes))
	}

	if volumes[0].VolumeID != "vol-123" {
		t.Errorf("VolumeID = %q", volumes[0].VolumeID)
	}
	if volumes[0].SizeGb != 50 {
		t.Errorf("Size = %q", volumes[0].SizeGb)
	}
	if volumes[0].StorageType != "gp3" {
		t.Errorf("StorageType = %q", volumes[0].StorageType)
	}
}

func TestEc2Service_GetVolumeDetails_Error(t *testing.T) {
	mock := &mockEc2API{
		volumesErr: errors.New("volume error"),
	}

	svc := &Ec2Service{api: mock}

	volumes, err := svc.GetVolumeDetails(context.Background(), []string{"vol-123"})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if volumes != nil {
		t.Errorf("expected nil volumes on error")
	}
}

func TestGetEbsVolumeData_Success(t *testing.T) {
	mock := &mockEc2API{
		volumesOutput: &ec2.DescribeVolumesOutput{
			Volumes: []types.Volume{
				{
					VolumeId:   aws.String("vol-111"),
					Size:       aws.Int32(30),
					VolumeType: types.VolumeTypeGp2,
				},
			},
		},
	}

	svc := &Ec2Service{api: mock}

	bdms := []types.InstanceBlockDeviceMapping{
		{
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId: aws.String("vol-111"),
			},
		},
	}

	volumes := svc.getEbsVolumeData(context.Background(), bdms)

	if len(volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(volumes))
	}

	if volumes[0].VolumeID != "vol-111" {
		t.Errorf("VolumeID = %q", volumes[0].VolumeID)
	}
}

func TestGetEbsVolumeData_EmptyInput(t *testing.T) {
	mock := &mockEc2API{}
	svc := &Ec2Service{api: mock}

	bdms := []types.InstanceBlockDeviceMapping{}

	volumes := svc.getEbsVolumeData(context.Background(), bdms)

	if len(volumes) != 0 {
		t.Errorf("expected 0 volumes, got %d", len(volumes))
	}
}

func TestGetEbsVolumeData_VolumeAPIError(t *testing.T) {
	mock := &mockEc2API{
		volumesErr: errors.New("volume api error"),
	}

	svc := &Ec2Service{api: mock}

	bdms := []types.InstanceBlockDeviceMapping{
		{
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId: aws.String("vol-999"),
			},
		},
	}

	volumes := svc.getEbsVolumeData(context.Background(), bdms)

	// Should return empty slice on error
	if len(volumes) != 0 {
		t.Errorf("expected 0 volumes on error, got %d", len(volumes))
	}
}
