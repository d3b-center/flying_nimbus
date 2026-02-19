package aws

import (
	"context"

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
