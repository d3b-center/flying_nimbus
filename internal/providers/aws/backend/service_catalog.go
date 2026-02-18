package aws

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
)

// ProvisionedProduct represents a provisioned product as returned by the Service Catalog API.
type ProvisionedProduct struct {
	ID                     string
	Name                   string
	Type                   string
	Status                 string
	StatusMessage          string
	CreatedTime            time.Time
	LastUpdatedTime        time.Time
	ProductID              string
	ProductName            string
	ProvisioningArtifactID string
	// Extra fields from API for detail view
	Arn                    string
	LastRecordID           string
	LastSuccessfulRecordID string
}

func (p ProvisionedProduct) Title() string {
	if p.Name != "" {
		return p.Name
	}
	return p.ID
}

func (p ProvisionedProduct) Description() string {
	status := fmt.Sprintf("🔴 %s", p.Status)
	if p.Status == "AVAILABLE" {
		status = fmt.Sprintf("🟢 %s", p.Status)
	}
	return status
}

func (p ProvisionedProduct) FilterValue() string { return p.Name }

type serviceCatalogAPI interface {
	ScanProvisionedProducts(ctx context.Context, params *servicecatalog.ScanProvisionedProductsInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.ScanProvisionedProductsOutput, error)
}

// ServiceCatalogService provides read-only access to AWS Service Catalog metadata.
type ServiceCatalogService struct {
	api serviceCatalogAPI
}

// InitServiceCatalogService initializes a new ServiceCatalogService using the provided AWS config.
func InitServiceCatalogService(cfg aws.Config) *ServiceCatalogService {
	slog.Debug("Initializing ServiceCatalog Service")
	return &ServiceCatalogService{
		api: servicecatalog.NewFromConfig(cfg),
	}
}

// ListProvisionedProducts returns all provisioned products.
func (s *ServiceCatalogService) ListProvisionedProducts(ctx context.Context) ([]ProvisionedProduct, error) {
	if s == nil || s.api == nil {
		return nil, fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	slog.Debug("Attempting to request provisioned products")
	output, err := s.api.ScanProvisionedProducts(ctx, &servicecatalog.ScanProvisionedProductsInput{})
	if err != nil {
		return nil, fmt.Errorf("Unable to load provisioned products: %v", err)
	}
	if output == nil {
		return []ProvisionedProduct{}, nil
	}

	products := make([]ProvisionedProduct, 0, len(output.ProvisionedProducts))
	for _, detail := range output.ProvisionedProducts {
		product := ProvisionedProduct{
			ID:                     aws.ToString(detail.Id),
			Name:                   aws.ToString(detail.Name),
			Type:                   aws.ToString(detail.Type),
			Status:                 string(detail.Status),
			StatusMessage:          aws.ToString(detail.StatusMessage),
			ProductID:              aws.ToString(detail.ProductId),
			ProvisioningArtifactID: aws.ToString(detail.ProvisioningArtifactId),
			Arn:                    aws.ToString(detail.Arn),
			LastRecordID:           aws.ToString(detail.LastRecordId),
			LastSuccessfulRecordID: aws.ToString(detail.LastSuccessfulProvisioningRecordId),
		}
		if detail.CreatedTime != nil {
			product.CreatedTime = *detail.CreatedTime
		}
		if detail.LastRecordId != nil {
			product.LastUpdatedTime = time.Now()
		}
		products = append(products, product)
	}
	return products, nil
}
