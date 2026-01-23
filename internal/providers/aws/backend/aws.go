package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

type AwsService struct {
	config *aws.Config
}

func InitAwsService(ctx context.Context) (*AwsService, error) {
	slog.Debug("Initializing AWS Service")
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("Unable to load SDK config: %v", err)
	}

	slog.Info(fmt.Sprintf("AWS Region: %s", cfg.Region))

	return &AwsService{
		config: &cfg,
	}, nil

}
