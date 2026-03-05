package aws

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/arn"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

type PrincipalType string

const (
	PrincipalUser        PrincipalType = "User"
	PrincipalAssumedRole PrincipalType = "AssumedRole"
	PrincipalRoot        PrincipalType = "Root"
)

type CallerIdentity struct {
	AccountID     string
	ARN           string
	RoleName      string
	SessionName   string
	PrincipalType PrincipalType
	Region        string
}

func (c CallerIdentity) WhoAmI() string {
	var whoAmI = "Account Unknown"
	if c.PrincipalType == PrincipalAssumedRole && c.AccountID != "" && c.Region != "" {
		whoAmI = fmt.Sprintf("%s (%s) / %s",
			c.RoleName,
			c.AccountID,
			c.Region,
		)
	}

	return whoAmI
}

// InitCallerIdentity fetches the caller identity once via STS and parses role/session fields.
func InitCallerIdentity(ctx context.Context, cfg aws.Config) (*CallerIdentity, error) {
	client := sts.NewFromConfig(cfg)

	timedCtx, cancel := context.WithTimeout(ctx, time.Second*3)
	defer cancel()

	out, err := client.GetCallerIdentity(timedCtx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return &CallerIdentity{}, fmt.Errorf("failed to get caller identity: %w", err)
	}

	id := &CallerIdentity{
		AccountID: aws.ToString(out.Account),
		ARN:       aws.ToString(out.Arn),
		Region:    cfg.Region,
	}

	if err := id.parseARN(); err != nil {
		return &CallerIdentity{}, err
	}

	return id, nil
}

func (id *CallerIdentity) parseARN() error {
	parsed, err := arn.Parse(id.ARN)
	if err != nil {
		return fmt.Errorf("invalid ARN: %w", err)
	}

	resourceParts := strings.Split(parsed.Resource, "/")

	if strings.HasPrefix(parsed.Resource, "assumed-role/") && len(resourceParts) >= 3 {
		// assumed-role/<role-name>/<session-name>
		id.PrincipalType = PrincipalAssumedRole
		id.RoleName = resourceParts[1]
		id.SessionName = resourceParts[2]
	} else {
		return fmt.Errorf("unsupported ARN format: %s", parsed.Resource)
	}

	return nil
}
