package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

type AwsService struct {
	config *aws.Config
}

func initAwsService(ctx context.Context) (*AwsService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("Unable to load SDK config: %v", err)
	}

	return &AwsService{
		config: &cfg,
	}, nil

}
