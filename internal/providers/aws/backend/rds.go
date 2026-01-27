package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

type RDSInstance struct {
	Id                   string
	Endpoint             string
	Port                 int32
	Status               string
	DbEngine             string
	DbVersion            string
	VpcID                string
	SubnetIds            []string
	InstanceClass        string
	VCPU                 int
	MemoryGiB            int
	AllocatedStorage     int32
	IsPubliclyAccessible bool
	SecurityGroupIds     []string
}

func (i RDSInstance) Title() string { return i.Id }

func (i RDSInstance) Description() string {
	status := fmt.Sprintf("🔴 %s", i.Status)

	if i.Status == "available" {
		status = fmt.Sprintf("🟢 %s", i.Status)
	}
	return status
}
func (i RDSInstance) FilterValue() string { return i.Id }

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
	slog.Debug(fmt.Sprintf("Total RDS Instances return %d", len(rawDBInstances.DBInstances)))

	for i, db := range rawDBInstances.DBInstances {
		instance := RDSInstance{
			Id:                   *db.DBInstanceIdentifier,
			Endpoint:             *db.Endpoint.Address,
			Port:                 *db.Endpoint.Port,
			Status:               *db.DBInstanceStatus,
			DbEngine:             *db.Engine,
			DbVersion:            *db.EngineVersion,
			InstanceClass:        *db.DBInstanceClass,
			VpcID:                *db.DBSubnetGroup.VpcId,
			IsPubliclyAccessible: *db.PubliclyAccessible,
			AllocatedStorage:     *db.AllocatedStorage,
			SecurityGroupIds:     getSecurityGroupIds(db.VpcSecurityGroups),
		}
		slog.Debug(fmt.Sprintf("DBs %v", instance))
		instances[i] = instance
	}

	return instances, nil
}

func getSecurityGroupIds(rawSGs []types.VpcSecurityGroupMembership) []string {
	securityGroups := make([]string, len(rawSGs))
	for i, sg := range rawSGs {
		securityGroups[i] = *sg.VpcSecurityGroupId
	}
	return securityGroups
}
