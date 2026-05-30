package aws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// IAMUser represents an IAM user with their access keys.
type IAMUser struct {
	UserName   string
	UserID     string
	Arn        string
	CreateDate time.Time
	AccessKeys []AccessKey
}

// AccessKey represents an IAM access key.
type AccessKey struct {
	AccessKeyID  string
	Status       string
	CreateDate   time.Time
	LastUsedDate *time.Time
	LastUsedBy   string
}

type iamAPI interface {
	ListUsers(ctx context.Context, params *iam.ListUsersInput, optFns ...func(*iam.Options)) (*iam.ListUsersOutput, error)
	ListAccessKeys(ctx context.Context, params *iam.ListAccessKeysInput, optFns ...func(*iam.Options)) (*iam.ListAccessKeysOutput, error)
	GetAccessKeyLastUsed(ctx context.Context, params *iam.GetAccessKeyLastUsedInput, optFns ...func(*iam.Options)) (*iam.GetAccessKeyLastUsedOutput, error)
}

// IamService provides methods for interacting with IAM users and access keys.
type IamService struct {
	api iamAPI
}

// InitIamService creates a new IAM service client.
func InitIamService(cfg aws.Config) *IamService {
	slog.Debug("Initializing IAM Service")
	client := iam.NewFromConfig(cfg)
	return &IamService{
		api: client,
	}
}

// ListUsersWithKeys retrieves all IAM users and their associated access keys.
func (s *IamService) ListUsersWithKeys(ctx context.Context) ([]IAMUser, error) {
	slog.Debug("Listing IAM users with access keys")

	var allUsers []IAMUser
	paginator := iam.NewListUsersPaginator(s.api, &iam.ListUsersInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list IAM users: %w", err)
		}

		for _, user := range page.Users {
			iamUser := IAMUser{
				UserName:   aws.ToString(user.UserName),
				UserID:     aws.ToString(user.UserId),
				Arn:        aws.ToString(user.Arn),
				CreateDate: aws.ToTime(user.CreateDate),
				AccessKeys: []AccessKey{},
			}

			// List access keys for this user
			keys, err := s.listAccessKeysForUser(ctx, iamUser.UserName)
			if err != nil {
				slog.Warn("Failed to list access keys for user", "user", iamUser.UserName, "error", err)
				// Continue with other users even if one fails
				continue
			}

			iamUser.AccessKeys = keys
			
			// Only include users that have at least one access key
			if len(iamUser.AccessKeys) > 0 {
				allUsers = append(allUsers, iamUser)
			}
		}
	}

	return allUsers, nil
}

// listAccessKeysForUser retrieves all access keys for a specific user.
func (s *IamService) listAccessKeysForUser(ctx context.Context, userName string) ([]AccessKey, error) {
	var keys []AccessKey

	paginator := iam.NewListAccessKeysPaginator(s.api, &iam.ListAccessKeysInput{
		UserName: aws.String(userName),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list access keys: %w", err)
		}

		for _, key := range page.AccessKeyMetadata {
			accessKey := AccessKey{
				AccessKeyID: aws.ToString(key.AccessKeyId),
				Status:      string(key.Status),
				CreateDate:  aws.ToTime(key.CreateDate),
			}

			// Get last used information
			lastUsed, err := s.api.GetAccessKeyLastUsed(ctx, &iam.GetAccessKeyLastUsedInput{
				AccessKeyId: key.AccessKeyId,
			})
			if err != nil {
				slog.Debug("Failed to get last used info for access key", "key", accessKey.AccessKeyID, "error", err)
			} else if lastUsed.AccessKeyLastUsed != nil {
				accessKey.LastUsedDate = lastUsed.AccessKeyLastUsed.LastUsedDate
				accessKey.LastUsedBy = aws.ToString(lastUsed.AccessKeyLastUsed.ServiceName)
			}

			keys = append(keys, accessKey)
		}
	}

	return keys, nil
}

// ListAllUsers retrieves all IAM users (without filtering by access keys).
func (s *IamService) ListAllUsers(ctx context.Context) ([]IAMUser, error) {
	slog.Debug("Listing all IAM users")

	var allUsers []IAMUser
	paginator := iam.NewListUsersPaginator(s.api, &iam.ListUsersInput{})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list IAM users: %w", err)
		}

		for _, user := range page.Users {
			iamUser := IAMUser{
				UserName:   aws.ToString(user.UserName),
				UserID:     aws.ToString(user.UserId),
				Arn:        aws.ToString(user.Arn),
				CreateDate: aws.ToTime(user.CreateDate),
				AccessKeys: []AccessKey{},
			}
			allUsers = append(allUsers, iamUser)
		}
	}

	return allUsers, nil
}
