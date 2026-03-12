package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
)

type cfnAPI interface {
	ListStackResources(ctx context.Context, params *cloudformation.ListStackResourcesInput, optFns ...func(*cloudformation.Options)) (*cloudformation.ListStackResourcesOutput, error)
}

// CloudFormationService provides minimal CloudFormation access for listing stack resources.
type CloudFormationService struct {
	api cfnAPI
}

// InitCloudFormationService creates a new CloudFormation service client.
func InitCloudFormationService(cfg aws.Config) *CloudFormationService {
	slog.Debug("Initializing CloudFormation Service")
	return &CloudFormationService{
		api: cloudformation.NewFromConfig(cfg),
	}
}

// ListStackResourcePhysicalIDs returns the PhysicalResourceId for each resource in the stack matching resourceType.
// stackARN is the CloudFormation stack ARN (e.g. arn:aws:cloudformation:region:account:stack/name/id).
func (c *CloudFormationService) ListStackResourcePhysicalIDs(ctx context.Context, stackARN, resourceType string) ([]string, error) {
	if c == nil || c.api == nil {
		return nil, fmt.Errorf("CloudFormation client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if stackARN == "" {
		return nil, fmt.Errorf("stack ARN is required")
	}

	var ids []string
	var nextToken *string
	for {
		input := &cloudformation.ListStackResourcesInput{
			StackName: aws.String(stackARN),
			NextToken: nextToken,
		}
		output, err := c.api.ListStackResources(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list stack resources: %w", err)
		}
		if output == nil {
			break
		}
		for _, sum := range output.StackResourceSummaries {
			if sum.ResourceType != nil && string(*sum.ResourceType) == resourceType && sum.PhysicalResourceId != nil {
				ids = append(ids, *sum.PhysicalResourceId)
			}
		}
		nextToken = output.NextToken
		if nextToken == nil || *nextToken == "" {
			break
		}
	}
	return ids, nil
}
