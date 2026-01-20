package aws

import (
	"context"

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

type ec2API interface {
	DescribeInstancesAPI(ctx context.Context, params *ec2.DescribeInstancesInput) (*ec2.DescribeInstancesOutput, error)
}

type EC2Service struct {
	api ec2API
}

func (e EC2Service) ListInstances(ctx context.Context) ([]EC2Instance, error) {
	return nil, nil
}
