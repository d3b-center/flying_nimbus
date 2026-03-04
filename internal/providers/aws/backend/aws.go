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
	CloudFormation *CloudFormationService
	ServiceCatalog *ServiceCatalogService
	Ssm            *SsmService
	Identity       *CallerIdentity
	LoggedIn       bool
}

func InitAwsService(ctx context.Context) (*AwsService, error) {
	slog.Debug("Initializing AWS Service")
	cfg, err := config.LoadDefaultConfig(ctx)

	if err != nil {
		return nil, fmt.Errorf("Unable to load SDK config: %v", err)
	}

	identity, err := InitCallerIdentity(ctx, cfg)
	var loggedIn = false
	if err == nil {
		loggedIn = true
	}

	slog.Info(fmt.Sprintf("AWS Region: %s", cfg.Region))
	ec2 := InitEc2Service(cfg)

	rds := InitRdsService(cfg)

	sg := InitSgService(cfg)
	cfn := InitCloudFormationService(cfg)
	ssm := InitSsmService(cfg)

	serviceCatalog := InitServiceCatalogService(cfg, ec2, cfn)

	return &AwsService{
		config:         &cfg,
		Ec2:            ec2,
		Rds:            rds,
		Sg:             sg,
		CloudFormation: cfn,
		ServiceCatalog: serviceCatalog,
		Ssm:            ssm,
		Identity:       identity,
		LoggedIn:       loggedIn,
	}, nil

}
