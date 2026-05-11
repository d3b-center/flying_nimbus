package aws

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/emr"
)

// EmrCluster is a simplified view of an Amazon EMR cluster.
type EmrCluster struct {
	Id          string
	Name        string
	State       string
	// LastJobTime is the start time of the most recent step.
	// Nil means the cluster has no steps or they could not be fetched.
	LastJobTime *time.Time
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

// ListClustersWithActivity fetches all clusters, enriches each with the start
// time of its most recent step, then sorts by that timestamp descending
// (most recently active first). Clusters with no steps appear at the bottom.
func (s *EmrService) ListClustersWithActivity(ctx context.Context) ([]EmrCluster, error) {
	clusters, err := s.ListClusters(ctx)
	if err != nil {
		return nil, err
	}

	// ListSteps returns steps in reverse order (newest first). Fetching the
	// first page and reading index 0 gives the most recent step without
	// iterating through all pages.
	for i, c := range clusters {
		out, err := s.api.ListSteps(ctx, &emr.ListStepsInput{
			ClusterId: aws.String(c.Id),
		})
		if err != nil {
			slog.Debug("emr: could not get last step", "id", c.Id, "err", err)
			continue
		}
		if len(out.Steps) > 0 {
			st := out.Steps[0]
			if st.Status != nil && st.Status.Timeline != nil && st.Status.Timeline.StartDateTime != nil {
				t := *st.Status.Timeline.StartDateTime
				clusters[i].LastJobTime = &t
			}
		}
	}

	// Sort: most recently active first; clusters with no steps sink to bottom.
	sort.Slice(clusters, func(i, j int) bool {
		ti := clusters[i].LastJobTime
		tj := clusters[j].LastJobTime
		if ti == nil && tj == nil {
			return clusters[i].Name < clusters[j].Name
		}
		if ti == nil {
			return false
		}
		if tj == nil {
			return true
		}
		return ti.After(*tj)
	})

	return clusters, nil
}

// GetRecentSteps fetches the first page of steps for a cluster (newest first,
// limited to max entries). It is used for tree-view display rather than full
// step listing. Use ListSteps for exhaustive results.
func (s *EmrService) GetRecentSteps(ctx context.Context, clusterId string, max int) ([]EmrStep, error) {
	out, err := s.api.ListSteps(ctx, &emr.ListStepsInput{
		ClusterId: aws.String(clusterId),
	})
	if err != nil {
		return nil, fmt.Errorf("list EMR steps: %w", err)
	}
	var steps []EmrStep
	for i, st := range out.Steps {
		if i >= max {
			break
		}
		if st.Id == nil || st.Name == nil {
			continue
		}
		state := ""
		if st.Status != nil {
			state = string(st.Status.State)
		}
		steps = append(steps, EmrStep{
			Id:    *st.Id,
			Name:  *st.Name,
			State: state,
		})
	}
	return steps, nil
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
