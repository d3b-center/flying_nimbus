package aws

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

type mockSgAPI struct {
	output *ec2.DescribeSecurityGroupsOutput
	err    error
}

func (m *mockSgAPI) DescribeSecurityGroups(
	ctx context.Context,
	params *ec2.DescribeSecurityGroupsInput,
	optFns ...func(*ec2.Options),
) (*ec2.DescribeSecurityGroupsOutput, error) {
	return m.output, m.err
}

func TestFlattenIpPermissions(t *testing.T) {
	tests := []struct {
		name     string
		perm     types.IpPermission
		isEgress bool
		want     []SecurityGroupRule
	}{
		{
			name: "single CIDR ingress rule",
			perm: types.IpPermission{
				FromPort:   aws.Int32(5432),
				ToPort:     aws.Int32(5432),
				IpProtocol: aws.String("tcp"),
				IpRanges: []types.IpRange{
					{
						CidrIp:      aws.String("10.0.0.0/16"),
						Description: aws.String("internal"),
					},
				},
			},
			isEgress: false,
			want: []SecurityGroupRule{
				{
					FromPort:    5432,
					ToPort:      5432,
					IpProtocol:  "tcp",
					IsEgress:    false,
					CidrIpv4:    "10.0.0.0/16",
					Description: "internal",
				},
			},
		},
		{
			name: "security group reference egress rule",
			perm: types.IpPermission{
				FromPort:   aws.Int32(0),
				ToPort:     aws.Int32(0),
				IpProtocol: aws.String("-1"),
				UserIdGroupPairs: []types.UserIdGroupPair{
					{
						GroupId:     aws.String("sg-123"),
						Description: aws.String("app tier"),
					},
				},
			},
			isEgress: true,
			want: []SecurityGroupRule{
				{
					FromPort:       0,
					ToPort:         0,
					IpProtocol:     "-1",
					IsEgress:       true,
					ReferencedSGId: "sg-123",
					Description:    "app tier",
				},
			},
		},
		{
			name: "mixed CIDR and SG reference",
			perm: types.IpPermission{
				FromPort:   aws.Int32(80),
				ToPort:     aws.Int32(80),
				IpProtocol: aws.String("tcp"),
				IpRanges: []types.IpRange{
					{CidrIp: aws.String("0.0.0.0/0")},
				},
				UserIdGroupPairs: []types.UserIdGroupPair{
					{GroupId: aws.String("sg-999")},
				},
			},
			isEgress: false,
			want: []SecurityGroupRule{
				{
					FromPort:   80,
					ToPort:     80,
					IpProtocol: "tcp",
					IsEgress:   false,
					CidrIpv4:   "0.0.0.0/0",
				},
				{
					FromPort:       80,
					ToPort:         80,
					IpProtocol:     "tcp",
					IsEgress:       false,
					ReferencedSGId: "sg-999",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := flattenIpPermissions(tt.perm, tt.isEgress)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("flattenIpPermissions() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestDescribeSecurityGroupRules(t *testing.T) {
	mockAPI := &mockSgAPI{
		output: &ec2.DescribeSecurityGroupsOutput{
			SecurityGroups: []types.SecurityGroup{
				{
					GroupId:     aws.String("sg-1"),
					Description: aws.String("db sg"),
					IpPermissions: []types.IpPermission{
						{
							FromPort:   aws.Int32(5432),
							ToPort:     aws.Int32(5432),
							IpProtocol: aws.String("tcp"),
							IpRanges: []types.IpRange{
								{CidrIp: aws.String("10.0.0.0/16")},
							},
						},
					},
					IpPermissionsEgress: []types.IpPermission{
						{
							FromPort:   aws.Int32(0),
							ToPort:     aws.Int32(0),
							IpProtocol: aws.String("-1"),
							UserIdGroupPairs: []types.UserIdGroupPair{
								{GroupId: aws.String("sg-egress")},
							},
						},
					},
				},
			},
		},
	}

	svc := &SgService{api: mockAPI}

	result, err := svc.DescribeSecurityGroupRules(context.Background(), []string{"sg-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sg, ok := result["sg-1"]
	if !ok {
		t.Fatalf("expected security group sg-1")
	}

	if sg.Description != "db sg" {
		t.Errorf("description = %q, want %q", sg.Description, "db sg")
	}

	if len(sg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(sg.Rules))
	}

	// spot-check one ingress + one egress
	var ingress, egress bool
	for _, r := range sg.Rules {
		if !r.IsEgress && r.CidrIpv4 == "10.0.0.0/16" {
			ingress = true
		}
		if r.IsEgress && r.ReferencedSGId == "sg-egress" {
			egress = true
		}
	}

	if !ingress || !egress {
		t.Errorf("expected both ingress and egress rules, got %#v", sg.Rules)
	}
}

func TestDescribeSecurityGroupRules_Error(t *testing.T) {
	mockAPI := &mockSgAPI{
		err: errors.New("boom"),
	}

	svc := &SgService{api: mockAPI}

	_, err := svc.DescribeSecurityGroupRules(context.Background(), []string{"sg-1"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
