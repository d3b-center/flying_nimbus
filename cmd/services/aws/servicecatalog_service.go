package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
)

type ServiceCatalogService struct {
	client *servicecatalog.Client
}

func NewServiceCatalogService(ctx context.Context) (*ServiceCatalogService, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to load SDK config: %w", err)
	}

	return &ServiceCatalogService{
		client: servicecatalog.NewFromConfig(cfg),
	}, nil
}

type Product struct {
	ProductID          string
	ProductName        string
	ProductType        string
	Owner              string
	ShortDescription   string
	Distributor        string
	SupportDescription string
	HasDefaultPath     bool
}

type ProvisioningArtifact struct {
	ID          string
	Name        string
	Description string
	CreatedTime time.Time
	Guidance    string
}

type ProvisioningParameter struct {
	Key                   string
	DefaultValue          string
	Description           string
	IsNoEcho              bool
	ParameterType         string
	ConstraintDescription string
	AllowedValues         []string
}

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
}

// ListProducts returns all available Service Catalog products
func (s *ServiceCatalogService) ListProducts(ctx context.Context) ([]Product, error) {
	input := &servicecatalog.SearchProductsAsAdminInput{}

	result, err := s.client.SearchProductsAsAdmin(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list products: %w", err)
	}

	var products []Product
	for _, productView := range result.ProductViewDetails {
		product := Product{
			ProductID:          aws.ToString(productView.ProductViewSummary.ProductId),
			ProductName:        aws.ToString(productView.ProductViewSummary.Name),
			ProductType:        string(productView.ProductViewSummary.Type),
			Owner:              aws.ToString(productView.ProductViewSummary.Owner),
			ShortDescription:   aws.ToString(productView.ProductViewSummary.ShortDescription),
			Distributor:        aws.ToString(productView.ProductViewSummary.Distributor),
			SupportDescription: aws.ToString(productView.ProductViewSummary.SupportDescription),
			HasDefaultPath:     productView.ProductViewSummary.HasDefaultPath,
		}
		products = append(products, product)
	}

	return products, nil
}

// ListProvisioningArtifacts returns all versions of a product
func (s *ServiceCatalogService) ListProvisioningArtifacts(ctx context.Context, productID string) ([]ProvisioningArtifact, error) {
	input := &servicecatalog.ListProvisioningArtifactsInput{
		ProductId: aws.String(productID),
	}

	result, err := s.client.ListProvisioningArtifacts(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list provisioning artifacts: %w", err)
	}

	var artifacts []ProvisioningArtifact
	for _, detail := range result.ProvisioningArtifactDetails {
		artifact := ProvisioningArtifact{
			ID:          aws.ToString(detail.Id),
			Name:        aws.ToString(detail.Name),
			Description: aws.ToString(detail.Description),
			Guidance:    string(detail.Guidance),
		}
		if detail.CreatedTime != nil {
			artifact.CreatedTime = *detail.CreatedTime
		}
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

// DescribeProvisioningParameters returns parameters needed to provision a product
func (s *ServiceCatalogService) DescribeProvisioningParameters(ctx context.Context, productID string, artifactID string) ([]ProvisioningParameter, error) {
	pathInput := &servicecatalog.ListLaunchPathsInput{
		ProductId: aws.String(productID),
	}

	pathResult, err := s.client.ListLaunchPaths(ctx, pathInput)
	if err != nil {
		fmt.Printf("ERROR: Failed to list launch paths: %v\n", err)
		return nil, fmt.Errorf("failed to list launch paths: %w", err)
	}

	if len(pathResult.LaunchPathSummaries) == 0 {
		return nil, fmt.Errorf("no launch paths available for this product")
	}

	pathID := pathResult.LaunchPathSummaries[0].Id
	// pathName := pathResult.LaunchPathSummaries[0].Name

	input := &servicecatalog.DescribeProvisioningParametersInput{
		ProductId:              aws.String(productID),
		ProvisioningArtifactId: aws.String(artifactID),
		PathId:                 pathID,
	}

	result, err := s.client.DescribeProvisioningParameters(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe provisioning parameters: %w", err)
	}

	var parameters []ProvisioningParameter
	for _, param := range result.ProvisioningArtifactParameters {
		parameter := ProvisioningParameter{
			Key:                   aws.ToString(param.ParameterKey),
			DefaultValue:          aws.ToString(param.DefaultValue),
			Description:           aws.ToString(param.Description),
			IsNoEcho:              param.IsNoEcho,
			ParameterType:         aws.ToString(param.ParameterType),
			ConstraintDescription: aws.ToString(param.ParameterConstraints.ConstraintDescription),
		}

		if param.ParameterConstraints != nil {
			parameter.AllowedValues = param.ParameterConstraints.AllowedValues
		}

		parameters = append(parameters, parameter)
	}

	return parameters, nil
}

// ProvisionProduct provisions a new product
func (s *ServiceCatalogService) ProvisionProduct(ctx context.Context, productID, artifactID, provisionedProductName string, parameters map[string]string) (string, error) {
	// Convert parameters map to ProvisioningParameter slice
	var provisioningParams []types.ProvisioningParameter
	for key, value := range parameters {
		provisioningParams = append(provisioningParams, types.ProvisioningParameter{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	// Generate unique token for idempotency
	token := fmt.Sprintf("provision-%d", time.Now().UnixNano())

	input := &servicecatalog.ProvisionProductInput{
		ProductId:              aws.String(productID),
		ProvisioningArtifactId: aws.String(artifactID),
		ProvisionedProductName: aws.String(provisionedProductName),
		ProvisioningParameters: provisioningParams,
		ProvisionToken:         aws.String(token),
	}

	result, err := s.client.ProvisionProduct(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to provision product: %w", err)
	}

	return aws.ToString(result.RecordDetail.RecordId), nil
}

// ListProvisionedProducts returns all provisioned products
func (s *ServiceCatalogService) ListProvisionedProducts(ctx context.Context) ([]ProvisionedProduct, error) {
	input := &servicecatalog.ScanProvisionedProductsInput{}

	result, err := s.client.ScanProvisionedProducts(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to list provisioned products: %w", err)
	}

	var products []ProvisionedProduct
	for _, detail := range result.ProvisionedProducts {
		product := ProvisionedProduct{
			ID:                     aws.ToString(detail.Id),
			Name:                   aws.ToString(detail.Name),
			Type:                   aws.ToString(detail.Type),
			Status:                 string(detail.Status),
			StatusMessage:          aws.ToString(detail.StatusMessage),
			ProductID:              aws.ToString(detail.ProductId),
			ProvisioningArtifactID: aws.ToString(detail.ProvisioningArtifactId),
		}

		if detail.CreatedTime != nil {
			product.CreatedTime = *detail.CreatedTime
		}
		if detail.LastRecordId != nil {
			product.LastUpdatedTime = time.Now() // You'd need to fetch this from record details
		}

		products = append(products, product)
	}

	return products, nil
}

// TerminateProvisionedProduct terminates a provisioned product
func (s *ServiceCatalogService) TerminateProvisionedProduct(ctx context.Context, provisionedProductID string) error {
	token := fmt.Sprintf("terminate-%d", time.Now().UnixNano())

	input := &servicecatalog.TerminateProvisionedProductInput{
		ProvisionedProductId: aws.String(provisionedProductID),
		TerminateToken:       aws.String(token),
	}

	_, err := s.client.TerminateProvisionedProduct(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to terminate provisioned product: %w", err)
	}

	return nil
}

// GetProvisioningRecordStatus gets the status of a provisioning operation
func (s *ServiceCatalogService) GetProvisioningRecordStatus(ctx context.Context, recordID string) (string, string, error) {
	input := &servicecatalog.DescribeRecordInput{
		Id: aws.String(recordID),
	}

	result, err := s.client.DescribeRecord(ctx, input)
	if err != nil {
		return "", "", fmt.Errorf("failed to describe record: %w", err)
	}

	status := string(result.RecordDetail.Status)
	statusMessage := ""

	if len(result.RecordDetail.RecordErrors) > 0 {
		statusMessage = aws.ToString(result.RecordDetail.RecordErrors[0].Description)
	}

	return status, statusMessage, nil
}
