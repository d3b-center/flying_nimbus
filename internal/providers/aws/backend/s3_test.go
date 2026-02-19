package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type mockS3API struct {
	listBucketsOutput *s3.ListBucketsOutput
	bucketsErr        error
	listObjectsOutput *s3.ListObjectsV2Output
	objectsErr        error
}

func (m *mockS3API) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFuncs ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if m.bucketsErr != nil {
		return nil, m.bucketsErr
	}
	return m.listBucketsOutput, nil
}

func (m *mockS3API) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFuncs ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.objectsErr != nil {
		return nil, m.objectsErr
	}
	return m.listObjectsOutput, nil
}
