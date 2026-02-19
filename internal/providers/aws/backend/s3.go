package aws

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Bucket struct {
	Name string
}

type s3API interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFuncs ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type S3Service struct {
	api s3API
}

func (b S3Bucket) Title() string {
	return b.Name
}

func (b S3Bucket) Description() string {
	return b.Name
}

func (b S3Bucket) FilterValue() string {
	return b.Name
}

func InitS3Service(cfg aws.Config) *S3Service {
	slog.Debug("Initializing S3 Service")
	client := s3.NewFromConfig(cfg)
	return &S3Service{
		api: client,
	}
}

func (s S3Service) ListBuckets(ctx context.Context) ([]S3Bucket, error) {
	results := []S3Bucket{}
	input := s3.ListBucketsInput{}
	for paginator := s3.NewListBucketsPaginator(s.api, &input); paginator.HasMorePages(); {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, bkt := range output.Buckets {
			if bkt.Name == nil {
				return nil, fmt.Errorf("Bucket has no name?")
			}
			results = append(results, S3Bucket{Name: *bkt.Name})
		}
	}
	return results, nil
}
