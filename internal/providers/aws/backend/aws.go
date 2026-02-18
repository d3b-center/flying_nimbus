package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

type AwsService struct {
	config         *aws.Config
	Ec2            *Ec2Service
	Rds            *RdsService
	Sg             *SgService
	ServiceCatalog *ServiceCatalogService
}

func InitAwsService(ctx context.Context) (*AwsService, error) {
	slog.Debug("Initializing AWS Service")
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("Unable to load SDK config: %v", err)
	}

	slog.Info(fmt.Sprintf("AWS Region: %s", cfg.Region))
	ec2 := InitEc2Service(cfg)

	rds := InitRdsService(cfg)
	serviceCatalog := InitServiceCatalogService(cfg)

	sg := InitSgService(cfg)

	return &AwsService{
		config:         &cfg,
		Ec2:            ec2,
		Rds:            rds,
		Sg:             sg,
		ServiceCatalog: serviceCatalog,
	}, nil

}
