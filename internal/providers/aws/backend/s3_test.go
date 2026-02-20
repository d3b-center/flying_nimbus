package aws

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type mockS3API struct {
	listBucketsOutput *s3.ListBucketsOutput
	bucketsErr        error
	listObjectsOutput *s3.ListObjectsV2Output
	objectsErr        error
}

func (m mockS3API) ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFuncs ...func(*s3.Options)) (*s3.ListBucketsOutput, error) {
	if m.bucketsErr != nil {
		return nil, m.bucketsErr
	}
	return m.listBucketsOutput, nil
}

func (m mockS3API) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFuncs ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.objectsErr != nil {
		return nil, m.objectsErr
	}
	return m.listObjectsOutput, nil
}

func TestS3Bucket_ListItemInterface(t *testing.T) {
	bkt := S3Bucket{Name: "bob"}

	if bkt.Title() != "bob" {
		t.Errorf("Title() = %q, want %q", bkt.Title(), "bob")
	}

	if bkt.Description() != "bob" {
		t.Errorf("Description() = %q, want %q", bkt.Description(), "bob")
	}

	if bkt.FilterValue() != "bob" {
		t.Errorf("FilterValue() = %q, want %q", bkt.FilterValue(), "bob")
	}
}

func TestS3Service_ListBucketsSuccess(t *testing.T) {
	bucket1Name := "bucket1"
	bucket2Name := "bucket2"
	mockApi := mockS3API{
		listBucketsOutput: &s3.ListBucketsOutput{
			Buckets: []s3Types.Bucket{
				s3Types.Bucket{
					Name: &bucket1Name,
				},
				s3Types.Bucket{
					Name: &bucket2Name,
				},
			},
		},
	}

	s3Service := S3Service{api: mockApi}

	buckets, err := s3Service.ListBuckets(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(buckets) != 2 {
		t.Errorf("expected 2 buckets, got %d", len(buckets))
	}
}

func TestS3Service_ListBucketsErr(t *testing.T) {
	mockApi := mockS3API{
		bucketsErr: fmt.Errorf("error 123"),
	}

	s3Service := S3Service{api: mockApi}

	buckets, err := s3Service.ListBuckets(context.Background())

	if buckets != nil {
		t.Errorf("returned buckets should be nil, got %v", buckets)
	}

	if err == nil {
		t.Errorf("err should be nil, got %v", err)
	}

	if err.Error() != "error 123" {
		t.Errorf("err.Error() should be \"error 123\", got %q", err.Error())
	}
}
