// Package awsclient performs different actions for ec2 service
package aws

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

type SSMService struct {
	client *ssm.Client
	config aws.Config
}

func NewSSMService(ctx context.Context) (*SSMService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &SSMService{
		client: ssm.NewFromConfig(cfg),
		config: cfg,
	}, nil
}

// IsSessionManagerPluginInstalled check if the session-manager-plugin is installed
func (s *SSMService) IsSessionManagerPluginInstalled() bool {
	_, err := exec.LookPath("session-manager-plugin")
	fmt.Print(err.Error())
	return err == nil
}

// IsInstanceOnline check if an instance is available for SSM sessions
func (s *SSMService) IsInstanceOnline(ctx context.Context, instanceID string) (bool, error) {
	input := &ssm.DescribeInstanceInformationInput{
		Filters: []types.InstanceInformationStringFilter{
			{
				Key:    aws.String("InstanceIds"),
				Values: []string{instanceID},
			},
		},
	}

	result, err := s.client.DescribeInstanceInformation(ctx, input)
	if err != nil {
		return false, fmt.Errorf("failed to check SSM status: %w", err)
	}

	if len(result.InstanceInformationList) == 0 {
		return false, nil
	}

	// Check if instance is online
	instance := result.InstanceInformationList[0]
	pingStatus := aws.ToString((*string)(&instance.PingStatus.Values()[0]))

	return pingStatus == "Online", nil
}

// StartSession Start an interactive SSM session
func (s *SSMService) StartSession(ctx context.Context, instanceID string) error {
	fmt.Println("Checking if SSM Plugin is installed on local machine...")

	//if !s.IsSessionManagerPluginInstalled() {
	//	fmt.Println("SSM Plugin is NOT installed on local machine...")
	//	return fmt.Errorf("session-manager-plugin is not installed.\n\n" +
	//		"Install it from:\n" +
	//}
	//		"https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html")

	fmt.Println("SSM Plugin found!")
	fmt.Printf("Starting session for instance: %s\n", instanceID)
	fmt.Printf("Region: %s\n\n", s.config.Region)

	// Build the command
	cmd := exec.CommandContext(ctx, "aws", "ssm", "start-session",
		"--target", instanceID,
		"--region", s.config.Region)

	// IMPORTANT: These must be set for interactive session
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run and capture error
	fmt.Println("Executing AWS SSM command...")
	err := cmd.Run()
	if err != nil {
		// Check for specific error types
		if exitError, ok := err.(*exec.ExitError); ok {
			fmt.Printf("\nCommand exited with code: %d\n", exitError.ExitCode())
		}
		fmt.Printf("\nError details: %v\n", err)
		return fmt.Errorf("SSM session failed: %w", err)
	}

	fmt.Println("\nSession completed successfully")
	return nil
}

// StartPortForwarding start a port forwarding session
func (s *SSMService) StartPortForwarding(ctx context.Context, instanceID string, localPort, remotePort int) error {
	if !s.IsSessionManagerPluginInstalled() {
		return fmt.Errorf("session-manager-plugin is not installed")
	}

	cmd := exec.CommandContext(ctx, "aws", "ssm", "start-session",
		"--target", instanceID,
		"--document-name", "AWS-StartPortForwardingSession",
		"--parameters", fmt.Sprintf(`{"portNumber":["%d"],"localPortNumber":["%d"]}`, remotePort, localPort),
		"--region", s.config.Region)

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
