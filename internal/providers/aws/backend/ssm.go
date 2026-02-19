// Package aws provides AWS service integrations.
package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SsmAPI defines the SSM API methods used by SsmService.
type SsmAPI interface {
	DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error)
}

// SsmService provides SSM session and port forwarding operations.
type SsmService struct {
	api    SsmAPI
	region string
}

// SsmSessionType represents the kind of SSM session to start.
type SsmSessionType int

const (
	// SessionTypeShell starts an interactive shell session.
	SessionTypeShell SsmSessionType = iota
	// SessionTypePortForward starts local port forwarding to the instance.
	SessionTypePortForward
	// SessionTypeRemotePortForward starts port forwarding to a remote host through the instance.
	SessionTypeRemotePortForward
)

type PortForwardConfig struct {
	LocalPort  int
	RemotePort int
	RemoteHost string // only used for remote port forwarding
}

// SsmInstanceStatus represents SSM connectivity status for an EC2 instance.
type SsmInstanceStatus struct {
	InstanceID   string
	IsManaged    bool
	PingStatus   string
	AgentVersion string
}

// NewSsmService creates a new SsmService.
func NewSsmService(api SsmAPI, region string) *SsmService {
	return &SsmService{api: api, region: region}
}

// GetInstanceStatus checks if an instance is managed by SSM.
func (s *SsmService) GetInstanceStatus(ctx context.Context, instanceID string) (*SsmInstanceStatus, error) {
	input := &ssm.DescribeInstanceInformationInput{
		Filters: []ssmtypes.InstanceInformationStringFilter{
			{
				Key:    aws.String("InstanceIds"),
				Values: []string{instanceID},
			},
		},
	}

	result, err := s.api.DescribeInstanceInformation(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instance info: %w", err)
	}

	status := &SsmInstanceStatus{
		InstanceID: instanceID,
		IsManaged:  false,
		PingStatus: "Unknown",
	}

	if len(result.InstanceInformationList) > 0 {
		info := result.InstanceInformationList[0]
		status.IsManaged = true
		status.PingStatus = string(info.PingStatus)
		status.AgentVersion = aws.ToString(info.AgentVersion)
	}

	return status, nil
}

// BuildSessionCmd builds an exec.Cmd for an interactive SSM shell session.
func (s *SsmService) BuildSessionCmd(instanceID string) *exec.Cmd {
	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--region", s.region,
	}
	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// BuildPortForwardCmd builds an exec.Cmd for SSM port forwarding to the instance.
func (s *SsmService) BuildPortForwardCmd(instanceID string, config PortForwardConfig) *exec.Cmd {
	params := fmt.Sprintf(`{"portNumber":["%d"],"localPortNumber":["%d"]}`,
		config.RemotePort, config.LocalPort)

	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", params,
		"--region", s.region,
	}
	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// BuildRemotePortForwardCmd builds an exec.Cmd for port forwarding to a remote host through the instance.
func (s *SsmService) BuildRemotePortForwardCmd(instanceID string, config PortForwardConfig) *exec.Cmd {
	params := fmt.Sprintf(`{"host":["%s"],"portNumber":["%d"],"localPortNumber":["%d"]}`,
		config.RemoteHost, config.RemotePort, config.LocalPort)

	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSessionToRemoteHost",
		"--parameters", params,
		"--region", s.region,
	}
	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// ValidatePort checks if a port string is a valid port number.
func ValidatePort(port string) (int, error) {
	p, err := strconv.Atoi(port)
	if err != nil {
		return 0, fmt.Errorf("invalid port number: %s", port)
	}
	if p < 1 || p > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535, got %d", p)
	}
	return p, nil
}