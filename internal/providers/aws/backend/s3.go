package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Bucket struct {
	Name string
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

type S3Object struct {
	Name string
}

// TODO don't think I need these methods, will use a different strategy to display these
func (o S3Object) Title() string {
	return o.Name
}

func (o S3Object) Description() string {
	return o.Name
}

func (o S3Object) FilterValue() string {
	return o.Name
}

type s3API interface {
	ListBuckets(ctx context.Context, params *s3.ListBucketsInput, optFuncs ...func(*s3.Options)) (*s3.ListBucketsOutput, error)
	ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
}

type S3Service struct {
	api s3API
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
				return nil, fmt.Errorf("bucket has no name?")
			}
			results = append(results, S3Bucket{Name: *bkt.Name})
		}
	}
	return results, nil
}

func (s S3Service) ListBucketObjects(ctx context.Context, bucketName string) ([]S3Object, error) {
	results := []S3Object{}
	input := s3.ListObjectsV2Input{
		Bucket: &bucketName,
	}
	for paginator := s3.NewListObjectsV2Paginator(s.api, &input, stoppingOnDupToken); paginator.HasMorePages(); {
		output, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range output.Contents {
			if obj.Key == nil {
				return nil, fmt.Errorf("s3 object has no key?")
			}
			results = append(results, S3Object{Name: *obj.Key})
		}
	}
	return results, nil
}

func stoppingOnDupToken(opt *s3.ListObjectsV2PaginatorOptions) {
	opt.StopOnDuplicateToken = true
}

type S3FileTree struct {
	Files   []string
	Subdirs map[string]*S3FileTree
}

func newS3FileTree() *S3FileTree {
	return &S3FileTree{
		Files:   make([]string, 0),
		Subdirs: make(map[string]*S3FileTree),
	}
}

func buildFileTree(objects []S3Object) *S3FileTree {
	tree := newS3FileTree()
	for _, obj := range objects {
		path := strings.Split(obj.Name, "/")
		tree.insertPath(path)
	}
	return tree
}

func (tree *S3FileTree) insertPath(path []string) {
	switch len(path) {
	case 0:
		return
	case 1:
		tree.Files = append(tree.Files, path[0])
	default:
		dirname := path[0]
		remainder := path[1:]
		dir, ok := tree.Subdirs[dirname]
		if !ok {
			dir = newS3FileTree()
			tree.Subdirs[dirname] = dir
		}
		dir.insertPath(remainder)
	}
}
