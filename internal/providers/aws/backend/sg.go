package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
)

type SecurityGroup struct {
	Id string
}

type sgAPI interface {
	DescribeSecurityGroupRules(ctx context.Context, params *ec2.DescribeSecurityGroupRulesInput, optFns ...func(*ec2.Options)) (*ec2.DescribeSecurityGroupRulesOutput, error)
}

type SgService struct {
	api sgAPI
}

func (s *SgService) DescribeSecurityGroupRules(ctx context.Context, sgIds []string) {

}
