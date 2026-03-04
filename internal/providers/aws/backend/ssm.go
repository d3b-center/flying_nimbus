// Package aws provides AWS service integrations.
package aws

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// SsmService provides SSM session and port forwarding operations.
type SsmService struct {
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

// InitSsmService creates a new SsmService.
func InitSsmService(cfg aws.Config) *SsmService {
	return &SsmService{
		region: cfg.Region,
	}
}

// BuildSessionCmd builds an exec.Cmd for an interactive SSM shell session.
func (s *SsmService) BuildSessionCmd(instanceID string) *exec.Cmd {
	args := []string{
		"ssm", "start-session",
		"--target", instanceID,
		"--region", s.region,
	}
	slog.Debug("SSM Command", "args", args)
	// Clear screen for UX
	fmt.Print("\033[2J\033[H")
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
func ValidatePort(port string) error {
	p, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number: %s", port)
	}
	if p < 1 || p > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", p)
	}
	return nil
}
