package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

type IAMService struct {
	client *iam.Client
}

func NewIAMService(ctx context.Context) (*IAMService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &IAMService{
		client: iam.NewFromConfig(cfg),
	}, nil
}

type InstanceProfileInfo struct {
	ProfileName      string
	ProfileArn       string
	RoleName         string
	RoleArn          string
	AttachedPolicies []PolicyInfo
	InlinePolicies   []string
}

type PolicyInfo struct {
	PolicyName string
	PolicyArn  string
	IsManaged  bool
}

// GetInstanceProfileInfo gets detailed information about an instance profile
func (s *IAMService) GetInstanceProfileInfo(ctx context.Context, profileName string) (*InstanceProfileInfo, error) {
	// Get instance profile
	profileResult, err := s.client.GetInstanceProfile(ctx, &iam.GetInstanceProfileInput{
		InstanceProfileName: aws.String(profileName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get instance profile: %w", err)
	}

	if len(profileResult.InstanceProfile.Roles) == 0 {
		return &InstanceProfileInfo{
			ProfileName: profileName,
			ProfileArn:  aws.ToString(profileResult.InstanceProfile.Arn),
		}, nil
	}

	// Get the role (usually only one role per instance profile)
	role := profileResult.InstanceProfile.Roles[0]
	roleName := aws.ToString(role.RoleName)

	info := &InstanceProfileInfo{
		ProfileName: profileName,
		ProfileArn:  aws.ToString(profileResult.InstanceProfile.Arn),
		RoleName:    roleName,
		RoleArn:     aws.ToString(role.Arn),
	}

	// Get attached managed policies
	attachedPoliciesResult, err := s.client.ListAttachedRolePolicies(ctx, &iam.ListAttachedRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list attached policies: %w", err)
	}

	for _, policy := range attachedPoliciesResult.AttachedPolicies {
		info.AttachedPolicies = append(info.AttachedPolicies, PolicyInfo{
			PolicyName: aws.ToString(policy.PolicyName),
			PolicyArn:  aws.ToString(policy.PolicyArn),
			IsManaged:  true,
		})
	}

	// Get inline policies
	inlinePoliciesResult, err := s.client.ListRolePolicies(ctx, &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list inline policies: %w", err)
	}

	info.InlinePolicies = inlinePoliciesResult.PolicyNames

	return info, nil
}
