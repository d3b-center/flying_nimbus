package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// Parameter represents an SSM Parameter Store entry.
type Parameter struct {
	Name         string
	ARN          string
	Desc         string
	Tags         map[string]string
	LastModified string
	Type         string
}

// Title returns the parameter name for list display.
func (p Parameter) Title() string { return p.Name }

// Description returns the parameter type for list display.
func (p Parameter) Description() string { return p.Type }

// FilterValue returns the parameter name for list filtering.
func (p Parameter) FilterValue() string { return p.Name }

type parameterStoreAPI interface {
	DescribeParameters(ctx context.Context, params *ssm.DescribeParametersInput, optFns ...func(*ssm.Options)) (*ssm.DescribeParametersOutput, error)
	GetParameter(ctx context.Context, params *ssm.GetParameterInput, optFns ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
	ListTagsForResource(ctx context.Context, params *ssm.ListTagsForResourceInput, optFns ...func(*ssm.Options)) (*ssm.ListTagsForResourceOutput, error)
}

// ParameterStoreService provides methods for interacting with AWS SSM Parameter Store.
type ParameterStoreService struct {
	api parameterStoreAPI
}

// InitParameterStoreService creates a new Parameter Store service client.
func InitParameterStoreService(cfg aws.Config) *ParameterStoreService {
	slog.Debug("Initializing Parameter Store Service")
	client := ssm.NewFromConfig(cfg)
	return &ParameterStoreService{api: client}
}

// ListParametersByOwner retrieves all parameters tagged with Owner=<ownerName>.
func (s ParameterStoreService) ListParametersByOwner(ctx context.Context, ownerName string) ([]Parameter, error) {
	input := &ssm.DescribeParametersInput{
		ParameterFilters: []types.ParameterStringFilter{
			{
				Key:    aws.String("tag:Owner"),
				Option: aws.String("Equals"),
				Values: []string{ownerName},
			},
		},
	}
	return s.paginatedDescribeParameters(ctx, input, true)
}

// ListAllParameters retrieves every parameter visible to the caller without
// any owner filter. Tags are not fetched during the list call (they are
// resolved lazily when the user inspects a specific parameter) to avoid
// the N+1 ListTagsForResource overhead on large accounts.
func (s ParameterStoreService) ListAllParameters(ctx context.Context) ([]Parameter, error) {
	return s.paginatedDescribeParameters(ctx, &ssm.DescribeParametersInput{}, false)
}

func (s ParameterStoreService) paginatedDescribeParameters(ctx context.Context, input *ssm.DescribeParametersInput, loadTags bool) ([]Parameter, error) {
	params := make([]Parameter, 0)
	for {
		page, nextToken, err := s.fetchParametersPage(ctx, input, loadTags)
		if err != nil {
			return nil, err
		}
		params = append(params, page...)
		input.NextToken = nextToken
		if nextToken == nil {
			break
		}
	}
	return params, nil
}

func (s ParameterStoreService) fetchParametersPage(ctx context.Context, input *ssm.DescribeParametersInput, loadTags bool) ([]Parameter, *string, error) {
	result, err := s.api.DescribeParameters(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	params := make([]Parameter, 0, len(result.Parameters))
	for _, entry := range result.Parameters {
		var tags map[string]string
		if loadTags {
			tags, err = s.fetchTags(ctx, aws.ToString(entry.Name))
			if err != nil {
				slog.Warn(fmt.Sprintf("Failed to fetch tags for parameter %s: %v", aws.ToString(entry.Name), err))
				tags = map[string]string{}
			}
		}
		params = append(params, Parameter{
			Name:         aws.ToString(entry.Name),
			ARN:          aws.ToString(entry.ARN),
			Desc:         aws.ToString(entry.Description),
			Tags:         tags,
			LastModified: formatParameterTime(entry.LastModifiedDate),
			Type:         string(entry.Type),
		})
	}
	return params, result.NextToken, nil
}

func (s ParameterStoreService) fetchTags(ctx context.Context, name string) (map[string]string, error) {
	out, err := s.api.ListTagsForResource(ctx, &ssm.ListTagsForResourceInput{
		ResourceId:   aws.String(name),
		ResourceType: types.ResourceTypeForTaggingParameter,
	})
	if err != nil {
		return nil, err
	}
	tags := make(map[string]string, len(out.TagList))
	for _, t := range out.TagList {
		if t.Key != nil && t.Value != nil {
			tags[*t.Key] = *t.Value
		}
	}
	return tags, nil
}

// FetchParameterFields retrieves the parameter value and returns all key/value pairs
// parsed from a JSON string, or a single "value" entry for plain strings.
func (s ParameterStoreService) FetchParameterFields(ctx context.Context, name string) (map[string]string, error) {
	out, err := s.api.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get parameter value: %w", err)
	}

	raw := aws.ToString(out.Parameter.Value)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		result := make(map[string]string, len(parsed))
		for k, v := range parsed {
			result[k] = fmt.Sprintf("%v", v)
		}
		return result, nil
	}

	slog.Warn("Parameter is not JSON; returning raw value under key 'value'")
	return map[string]string{"value": raw}, nil
}

func formatParameterTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
