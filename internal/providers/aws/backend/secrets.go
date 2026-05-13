package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// Secret represents an AWS Secrets Manager secret entry.
type Secret struct {
	Name         string
	ARN          string
	Desc         string
	Tags         map[string]string
	LastChanged  string
	LastAccessed string
}

// Title returns the secret name for list display.
func (s Secret) Title() string { return s.Name }

// Description returns the secret ARN for list display.
func (s Secret) Description() string { return s.ARN }

// FilterValue returns the secret name for list filtering.
func (s Secret) FilterValue() string { return s.Name }

type secretsManagerAPI interface {
	ListSecrets(ctx context.Context, params *secretsmanager.ListSecretsInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.ListSecretsOutput, error)
	GetSecretValue(ctx context.Context, params *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// SecretsService provides methods for interacting with AWS Secrets Manager.
type SecretsService struct {
	api secretsManagerAPI
}

// InitSecretsService creates a new Secrets Manager service client.
func InitSecretsService(cfg aws.Config) *SecretsService {
	slog.Debug("Initializing Secrets Manager Service")
	client := secretsmanager.NewFromConfig(cfg)
	return &SecretsService{api: client}
}

// ListSecretsByOwner retrieves all secrets tagged with Owner=<ownerName>.
func (s SecretsService) ListSecretsByOwner(ctx context.Context, ownerName string) ([]Secret, error) {
	input := &secretsmanager.ListSecretsInput{
		Filters: []types.Filter{
			{Key: types.FilterNameStringTypeTagKey, Values: []string{"Owner"}},
			{Key: types.FilterNameStringTypeTagValue, Values: []string{ownerName}},
		},
	}
	return s.paginatedListSecrets(ctx, input)
}

// ListAllSecrets retrieves every secret visible to the caller without any
// owner filter.
func (s SecretsService) ListAllSecrets(ctx context.Context) ([]Secret, error) {
	return s.paginatedListSecrets(ctx, &secretsmanager.ListSecretsInput{})
}

// FetchSecretFields retrieves the secret value by ARN and returns all key/value pairs
// parsed from a JSON secret string. If the value is not JSON, returns a single entry
// keyed "value" containing the raw string.
func (s SecretsService) FetchSecretFields(ctx context.Context, arn string) (map[string]string, error) {
	out, err := s.api.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(arn),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get secret value: %w", err)
	}

	raw := aws.ToString(out.SecretString)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		result := make(map[string]string, len(parsed))
		for k, v := range parsed {
			result[k] = fmt.Sprintf("%v", v)
		}
		return result, nil
	}

	slog.Warn("Secret is not JSON; returning raw value under key 'value'")
	return map[string]string{"value": raw}, nil
}

func (s SecretsService) paginatedListSecrets(ctx context.Context, input *secretsmanager.ListSecretsInput) ([]Secret, error) {
	secrets := make([]Secret, 0)
	for {
		page, nextToken, err := s.fetchSecretsPage(ctx, input)
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, page...)
		input.NextToken = nextToken
		if nextToken == nil {
			break
		}
	}
	return secrets, nil
}

func (s SecretsService) fetchSecretsPage(ctx context.Context, input *secretsmanager.ListSecretsInput) ([]Secret, *string, error) {
	result, err := s.api.ListSecrets(ctx, input)
	if err != nil {
		return nil, nil, err
	}

	secrets := make([]Secret, 0, len(result.SecretList))
	for _, entry := range result.SecretList {
		secrets = append(secrets, Secret{
			Name:         aws.ToString(entry.Name),
			ARN:          aws.ToString(entry.ARN),
			Desc:         aws.ToString(entry.Description),
			Tags:         extractSecretTags(entry.Tags),
			LastChanged:  formatSecretTime(entry.LastChangedDate),
			LastAccessed: formatSecretTime(entry.LastAccessedDate),
		})
	}
	return secrets, result.NextToken, nil
}

// extractSecretTags converts Secrets Manager tags to a map.
func extractSecretTags(tags []types.Tag) map[string]string {
	tagMap := make(map[string]string)
	for _, tag := range tags {
		if tag.Key != nil && tag.Value != nil {
			tagMap[*tag.Key] = *tag.Value
		}
	}
	return tagMap
}

// formatSecretTime formats an optional time pointer as RFC3339, or "-" if nil.
func formatSecretTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.Format(time.RFC3339)
}
