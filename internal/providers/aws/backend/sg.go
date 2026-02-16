package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// SecurityGroup represents an AWS EC2 Security Group with its
// flattened ingress and egress rules.
type SecurityGroup struct {
	Id          string
	Description string
	Rules       []SecurityGroupRule
}

// SecurityGroupRule represents a single flattened security group rule.
// A rule corresponds to exactly one source:
//   - either a CIDR range (CidrIpv4)
//   - or a referenced security group (ReferencedSGId)
//
// Only one of CidrIpv4 or ReferencedSGId should be set.
type SecurityGroupRule struct {
	Id             string
	CidrIpv4       string
	Description    string
	FromPort       int32
	ToPort         int32
	IpProtocol     string // IpProtocol is the IP protocol (e.g. tcp, udp, -1).
	IsEgress       bool
	ReferencedSGId string
}

// sgAPI abstracts the EC2 client for describing security groups.
type sgAPI interface {
	DescribeSecurityGroups(ctx context.Context, params *ec2.DescribeSecurityGroupsInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupsOutput, error)
}

// SgService provides operations for retrieving and transforming
// EC2 Security Group data into domain models.
type SgService struct {
	api sgAPI
}

// InitSgService initializes a new SgService using the provided AWS config.
func InitSgService(cfg aws.Config) *SgService {
	slog.Debug("Initializing SecurityGroup Service")
	return &SgService{
		api: ec2.NewFromConfig(cfg),
	}
}

// DescribeSecurityGroupRules retrieves security groups by ID and returns
// a map keyed by security group ID containing flattened ingress and egress rules.
//
// Each IpPermission is expanded into one or more SecurityGroupRule entries
// so that each rule corresponds to exactly one CIDR range or security group reference.
func (s *SgService) DescribeSecurityGroupRules(ctx context.Context, sgIds []string) (map[string]*SecurityGroup, error) {
	result := make(map[string]*SecurityGroup)

	// TODO: Utilize NewDescribeSecurityGroupsPaginator
	output, err := s.api.DescribeSecurityGroups(ctx, &ec2.DescribeSecurityGroupsInput{
		GroupIds: sgIds,
	})

	if err != nil {
		slog.Error("Failed to request DescribeSecurityGroups", "error", err)
		return result, err
	}

	for _, sg := range output.SecurityGroups {
		sgId := aws.ToString(sg.GroupId)

		domainSG := &SecurityGroup{
			Id:          sgId,
			Description: aws.ToString(sg.Description),
			Rules:       []SecurityGroupRule{},
		}

		slog.Debug(
			"Processing security group",
			"id", sgId,
			"ingressPermissions", len(sg.IpPermissions),
			"egressPermissions", len(sg.IpPermissionsEgress),
		)

		for _, perm := range sg.IpPermissions { // ingress
			domainSG.Rules = append(
				domainSG.Rules,
				flattenIpPermissions(perm, false)...,
			)
		}

		for _, perm := range sg.IpPermissionsEgress { // egress
			domainSG.Rules = append(
				domainSG.Rules,
				flattenIpPermissions(perm, true)...,
			)
		}

		slog.Debug(fmt.Sprintf("RDS ID %s total rules %d", domainSG.Id, len(domainSG.Rules)))
		result[*sg.GroupId] = domainSG
	}

	return result, nil
}

// flattenIpPermissions expands a single EC2 IpPermission into one or more
// SecurityGroupRule entries.
//
// AWS allows a single IpPermission to reference multiple CIDR ranges
// and multiple security group references. Each source is flattened into
// its own rule to avoid losing information.
func flattenIpPermissions(perm types.IpPermission, isEgress bool) []SecurityGroupRule {
	var rules []SecurityGroupRule

	for _, ipRange := range perm.IpRanges {
		if ipRange.CidrIp == nil {
			continue
		}

		rule := SecurityGroupRule{
			FromPort:    aws.ToInt32(perm.FromPort),
			ToPort:      aws.ToInt32(perm.ToPort),
			IpProtocol:  aws.ToString(perm.IpProtocol),
			IsEgress:    isEgress,
			CidrIpv4:    aws.ToString(ipRange.CidrIp),
			Description: aws.ToString(ipRange.Description),
		}
		rules = append(rules, rule)
	}

	for _, sgRef := range perm.UserIdGroupPairs {
		if sgRef.GroupId == nil {
			continue
		}
		rule := SecurityGroupRule{
			FromPort:       aws.ToInt32(perm.FromPort),
			ToPort:         aws.ToInt32(perm.ToPort),
			IpProtocol:     aws.ToString(perm.IpProtocol),
			IsEgress:       isEgress,
			ReferencedSGId: aws.ToString(sgRef.GroupId),
			Description:    aws.ToString(sgRef.Description),
		}
		rules = append(rules, rule)
	}

	return rules
}
