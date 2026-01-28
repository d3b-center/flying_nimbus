package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

// RDSInstance represents a simplified, UI-friendly view of an AWS RDS instance.
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

// rdsAPI abstracts the AWS RDS API used by RdsService.
type rdsAPI interface {
	DescribeDBInstances(ctx context.Context, params *rds.DescribeDBInstancesInput, opts ...func(*rds.Options)) (*rds.DescribeDBInstancesOutput, error)
}

// RdsService provides read-only access to AWS RDS metadata.
type RdsService struct {
	api rdsAPI
}

func InitRdsService(cfg aws.Config) *RdsService {
	slog.Debug("Initializing RDS Service")
	return &RdsService{
		api: rds.NewFromConfig(cfg),
	}
}

// ListDBInstances returns all RDS DB instances visible to the caller.
// The results are mapped into domain-friendly RDSInstance structs.
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
			Id:                   aws.ToString(db.DBInstanceIdentifier),
			Endpoint:             aws.ToString(db.Endpoint.Address),
			Port:                 aws.ToInt32(db.Endpoint.Port),
			Status:               aws.ToString(db.DBInstanceStatus),
			DbEngine:             aws.ToString(db.Engine),
			DbVersion:            aws.ToString(db.EngineVersion),
			InstanceClass:        aws.ToString(db.DBInstanceClass),
			VpcID:                aws.ToString(db.DBSubnetGroup.VpcId),
			IsPubliclyAccessible: aws.ToBool(db.PubliclyAccessible),
			AllocatedStorage:     aws.ToInt32(db.AllocatedStorage),
			SecurityGroupIds:     getSecurityGroupIds(db.VpcSecurityGroups),
		}
		slog.Debug(fmt.Sprintf("DBs %v", instance))
		instances[i] = instance
	}

	return instances, nil
}

// getSecurityGroupIds extracts security group IDs from
// VpcSecurityGroupMembership records.
func getSecurityGroupIds(rawSGs []types.VpcSecurityGroupMembership) []string {
	securityGroups := make([]string, len(rawSGs))
	for i, sg := range rawSGs {
		securityGroups[i] = aws.ToString(sg.VpcSecurityGroupId)
	}
	return securityGroups
}
