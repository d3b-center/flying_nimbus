package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
)

// EmrCluster is a simplified view of an Amazon EMR cluster.
type EmrCluster struct {
	Id    string
	Name  string
	State string
}

// EmrStep is a simplified view of an EMR cluster step (job).
type EmrStep struct {
	Id    string
	Name  string
	State string
}

type emrAPI interface {
	ListClusters(
		ctx context.Context,
		params *emr.ListClustersInput,
		optFns ...func(*emr.Options),
	) (*emr.ListClustersOutput, error)

	ListSteps(
		ctx context.Context,
		params *emr.ListStepsInput,
		optFns ...func(*emr.Options),
	) (*emr.ListStepsOutput, error)
}

// EmrService provides read-only access to Amazon EMR cluster and step metadata.
type EmrService struct {
	api emrAPI
}

// InitEmrService creates a new EmrService from an AWS config.
func InitEmrService(cfg aws.Config) *EmrService {
	slog.Debug("Initializing EMR Service")
	return &EmrService{api: emr.NewFromConfig(cfg)}
}

// ListClusters returns all EMR clusters visible to the caller, collecting all
// pages. Includes clusters in all lifecycle states.
func (s *EmrService) ListClusters(ctx context.Context) ([]EmrCluster, error) {
	var clusters []EmrCluster
	var marker *string

	for {
		out, err := s.api.ListClusters(ctx, &emr.ListClustersInput{
			Marker: marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list EMR clusters: %w", err)
		}

		for _, c := range out.Clusters {
			if c.Id == nil || c.Name == nil {
				continue
			}
			state := ""
			if c.Status != nil {
				state = string(c.Status.State)
			}
			clusters = append(clusters, EmrCluster{
				Id:    *c.Id,
				Name:  *c.Name,
				State: state,
			})
		}

		if out.Marker == nil {
			break
		}
		marker = out.Marker
	}

	slog.Debug(fmt.Sprintf("emr: found %d clusters", len(clusters)))
	return clusters, nil
}

// ListSteps returns all steps for the given cluster, collecting all pages.
func (s *EmrService) ListSteps(ctx context.Context, clusterId string) ([]EmrStep, error) {
	var steps []EmrStep
	var marker *string

	for {
		out, err := s.api.ListSteps(ctx, &emr.ListStepsInput{
			ClusterId: aws.String(clusterId),
			Marker:    marker,
		})
		if err != nil {
			return nil, fmt.Errorf("list EMR steps: %w", err)
		}

		for _, s := range out.Steps {
			if s.Id == nil || s.Name == nil {
				continue
			}
			state := ""
			if s.Status != nil {
				state = string(s.Status.State)
			}
			steps = append(steps, EmrStep{
				Id:    *s.Id,
				Name:  *s.Name,
				State: state,
			})
		}

		if out.Marker == nil {
			break
		}
		marker = out.Marker
	}

	return steps, nil
}

// FindClusterByName searches for a cluster by exact name, then case-insensitive
// match, then partial match.
func (s *EmrService) FindClusterByName(ctx context.Context, name string) (EmrCluster, error) {
	clusters, err := s.ListClusters(ctx)
	if err != nil {
		return EmrCluster{}, err
	}

	lower := strings.ToLower(name)
	for _, c := range clusters {
		if c.Name == name {
			return c, nil
		}
	}
	for _, c := range clusters {
		if strings.EqualFold(c.Name, name) {
			return c, nil
		}
	}
	for _, c := range clusters {
		if strings.Contains(strings.ToLower(c.Name), lower) {
			return c, nil
		}
	}
	return EmrCluster{}, fmt.Errorf("no EMR cluster found matching %q", name)
}
