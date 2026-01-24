package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
)

type RDSInstance struct {
	Id       string
	Hostname string
	Port     int
	Status   string
	DbEngine string
}

func (i RDSInstance) Title() string { return i.Id }

func (i RDSInstance) Description() string {
	return fmt.Sprintf("Status %s Engine %s", i.Status, i.DbEngine)
}
func (i RDSInstance) FilterValue() string { return i.Id }

type InstanceConfig struct {
	InstanceClass string
	VCPU          int
	MemoryGiB     int
}

type NetworkConfig struct {
	VpcID     string
	SubnetIds []string
}

type rdsAPI interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

type RdsService struct {
	api rdsAPI
}

func InitRdsService(cfg aws.Config) *RdsService {
	slog.Debug("Initializing RDS Service")
	client := rds.NewFromConfig(cfg)
	return &RdsService{
		api: client,
	}
}

func (s *RdsService) ListDBInstances(ctx context.Context) ([]RDSInstance, error) {
	slog.Debug("Attempting to request DBs")
	rawDBInstances, err := s.api.DescribeDBInstances(ctx, &rds.DescribeDBInstancesInput{})

	if err != nil {
		return nil, fmt.Errorf("Unable to load RDS instances: %v", err)
	}

	instances := make([]RDSInstance, len(rawDBInstances.DBInstances))
	slog.Debug(fmt.Sprintf("Total DBs return %d", len(rawDBInstances.DBInstances)))

	for i, db := range rawDBInstances.DBInstances {
		slog.Debug(fmt.Sprintf("DB return %s", *db.DBInstanceIdentifier))
		instance := RDSInstance{
			Id:       *db.DBInstanceIdentifier,
			Hostname: *db.Endpoint.Address,
			Port:     int(*db.Endpoint.Port),
			Status:   *db.DBInstanceStatus,
			DbEngine: *db.Engine,
		}
		slog.Debug(fmt.Sprintf("DBs %v", instance))
		instances[i] = instance
	}

	return instances, nil
}
