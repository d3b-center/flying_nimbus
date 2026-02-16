package aws

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/rds/types"
)

type mockRdsAPI struct {
	output *rds.DescribeDBInstancesOutput
	err    error
}

func (m *mockRdsAPI) DescribeDBInstances(
	ctx context.Context,
	params *rds.DescribeDBInstancesInput,
	opts ...func(*rds.Options),
) (*rds.DescribeDBInstancesOutput, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.output, nil
}

func TestRDSInstance_ListItemInterface(t *testing.T) {
	inst := RDSInstance{
		Id:     "db-1",
		Status: "available",
	}

	if inst.Title() != "db-1" {
		t.Errorf("Title() = %q, want %q", inst.Title(), "db-1")
	}

	if inst.FilterValue() != "db-1" {
		t.Errorf("FilterValue() = %q, want %q", inst.FilterValue(), "db-1")
	}

	desc := inst.Description()
	if !strings.Contains(desc, "🟢") {
		t.Errorf("expected green status emoji, got %q", desc)
	}
}

func TestRDSInstance_Description_NotAvailable(t *testing.T) {
	inst := RDSInstance{
		Status: "stopped",
	}

	desc := inst.Description()
	if !strings.Contains(desc, "🔴") {
		t.Errorf("expected red status emoji, got %q", desc)
	}
}

func TestRdsService_ListDBInstances_Success(t *testing.T) {
	mock := &mockRdsAPI{
		output: &rds.DescribeDBInstancesOutput{
			DBInstances: []types.DBInstance{
				{
					DBInstanceIdentifier: aws.String("db-1"),
					DBInstanceStatus:     aws.String("available"),
					Engine:               aws.String("postgres"),
					EngineVersion:        aws.String("14.8"),
					DBInstanceClass:      aws.String("db.t3.medium"),
					PubliclyAccessible:   aws.Bool(false),
					AllocatedStorage:     aws.Int32(100),
					Endpoint: &types.Endpoint{
						Address: aws.String("db.example.com"),
						Port:    aws.Int32(5432),
					},
					DBSubnetGroup: &types.DBSubnetGroup{
						VpcId: aws.String("vpc-123"),
					},
					VpcSecurityGroups: []types.VpcSecurityGroupMembership{
						{VpcSecurityGroupId: aws.String("sg-1")},
						{VpcSecurityGroupId: aws.String("sg-2")},
					},
				},
			},
		},
	}

	svc := &RdsService{api: mock}

	instances, err := svc.ListDBInstances(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}

	inst := instances[0]

	if inst.Id != "db-1" {
		t.Errorf("Id = %q", inst.Id)
	}
	if inst.Endpoint != "db.example.com" {
		t.Errorf("Endpoint = %q", inst.Endpoint)
	}
	if inst.Port != 5432 {
		t.Errorf("Port = %d", inst.Port)
	}
	if inst.VpcID != "vpc-123" {
		t.Errorf("VpcID = %q", inst.VpcID)
	}
	if len(inst.SecurityGroupIds) != 2 {
		t.Errorf("expected 2 SGs, got %d", len(inst.SecurityGroupIds))
	}
}

func TestRdsService_ListDBInstances_Error(t *testing.T) {
	mock := &mockRdsAPI{
		err: errors.New("boom"),
	}

	svc := &RdsService{api: mock}

	instances, err := svc.ListDBInstances(context.Background())

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if instances != nil {
		t.Errorf("expected nil instances on error")
	}
}

func TestGetSecurityGroupIds(t *testing.T) {
	raw := []types.VpcSecurityGroupMembership{
		{VpcSecurityGroupId: aws.String("sg-1")},
		{VpcSecurityGroupId: aws.String("sg-2")},
	}

	ids := getSecurityGroupIds(raw)

	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	if ids[0] != "sg-1" || ids[1] != "sg-2" {
		t.Errorf("unexpected ids: %v", ids)
	}
}
