package aws

import "context"

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

type EC2Service interface {
	ListInstances(ctx context.Context) ([]EC2Instance, error)
	StartInstance(ctx context.Context, instanceID string) error
	StopInstance(ctx context.Context, instanceID string) error
	TerminateInstance(ctx context.Context, instanceID string) error
	GetInstanceDetails(ctx context.Context, instanceID string) (*EC2Instance, error)
}
