package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog"
	"github.com/aws/aws-sdk-go-v2/service/servicecatalog/types"
)

// Product represents a catalog product (available to provision).
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

func (p Product) Title() string {
	if p.ProductName != "" {
		return p.ProductName
	}
	return p.ProductID
}

func (p Product) Description() string {
	if len(p.ShortDescription) > 60 {
		return p.ShortDescription[:57] + "..."
	}
	return p.ShortDescription
}

func (p Product) FilterValue() string { return p.ProductName }

// ProvisioningArtifact represents a product version (for version selector).
type ProvisioningArtifact struct {
	ID          string
	Name        string
	Description string
	CreatedTime time.Time
	Guidance    string
}

// ProvisioningParameter represents a parameter for the provisioning form.
type ProvisioningParameter struct {
	Key                   string
	DefaultValue          string
	Description           string
	IsNoEcho              bool
	ParameterType         string
	ConstraintDescription string
	AllowedValues         []string
	Required              bool
}

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
	SearchProvisionedProducts(ctx context.Context, params *servicecatalog.SearchProvisionedProductsInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.SearchProvisionedProductsOutput, error)
	DescribeRecord(ctx context.Context, params *servicecatalog.DescribeRecordInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.DescribeRecordOutput, error)
	SearchProducts(ctx context.Context, params *servicecatalog.SearchProductsInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.SearchProductsOutput, error)
	DescribeProduct(ctx context.Context, params *servicecatalog.DescribeProductInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.DescribeProductOutput, error)
	ListLaunchPaths(ctx context.Context, params *servicecatalog.ListLaunchPathsInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.ListLaunchPathsOutput, error)
	DescribeProvisioningParameters(ctx context.Context, params *servicecatalog.DescribeProvisioningParametersInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.DescribeProvisioningParametersOutput, error)
	ProvisionProduct(ctx context.Context, params *servicecatalog.ProvisionProductInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.ProvisionProductOutput, error)
	TerminateProvisionedProduct(ctx context.Context, params *servicecatalog.TerminateProvisionedProductInput, optFns ...func(*servicecatalog.Options)) (*servicecatalog.TerminateProvisionedProductOutput, error)
}

// ServiceCatalogService provides access to AWS Service Catalog (catalog products, provision, provisioned products).
type ServiceCatalogService struct {
	api serviceCatalogAPI
	ec2 *Ec2Service
	cfn *CloudFormationService
}

// InitServiceCatalogService initializes a new ServiceCatalogService using the provided AWS config and optional EC2/CloudFormation for start/stop.
func InitServiceCatalogService(cfg aws.Config, ec2 *Ec2Service, cfn *CloudFormationService) *ServiceCatalogService {
	slog.Debug("Initializing ServiceCatalog Service")
	return &ServiceCatalogService{
		api: servicecatalog.NewFromConfig(cfg),
		ec2: ec2,
		cfn: cfn,
	}
}

// ListProvisionedProducts returns provisioned products accessible to the current user.
func (s *ServiceCatalogService) ListProvisionedProducts(ctx context.Context) ([]ProvisionedProduct, error) {
	if s == nil || s.api == nil {
		return nil, fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var products []ProvisionedProduct
	var pageToken *string
	for {
		input := &servicecatalog.SearchProvisionedProductsInput{
			AccessLevelFilter: &types.AccessLevelFilter{
				Key:   types.AccessLevelFilterKeyUser,
				Value: aws.String("self"),
			},
			PageToken: pageToken,
		}
		slog.Debug("Attempting to request provisioned products (user-only)")
		output, err := s.api.SearchProvisionedProducts(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("load provisioned products: %w", err)
		}
		if output == nil {
			break
		}
		for _, attr := range output.ProvisionedProducts {
			product := ProvisionedProduct{
				ID:                     aws.ToString(attr.Id),
				Name:                   aws.ToString(attr.Name),
				Type:                   aws.ToString(attr.Type),
				Status:                 string(attr.Status),
				StatusMessage:          aws.ToString(attr.StatusMessage),
				ProductID:              aws.ToString(attr.ProductId),
				ProductName:            aws.ToString(attr.ProductName),
				ProvisioningArtifactID: aws.ToString(attr.ProvisioningArtifactId),
				Arn:                    aws.ToString(attr.Arn),
				LastRecordID:           aws.ToString(attr.LastRecordId),
				LastSuccessfulRecordID: aws.ToString(attr.LastSuccessfulProvisioningRecordId),
			}
			if attr.CreatedTime != nil {
				product.CreatedTime = *attr.CreatedTime
			}
			if attr.LastRecordId != nil {
				product.LastUpdatedTime = time.Now()
			}
			products = append(products, product)
		}
		pageToken = output.NextPageToken
		if pageToken == nil || *pageToken == "" {
			break
		}
	}
	return products, nil
}

// ListCatalogProducts returns catalog products accessible to the current user based on portfolio grants (paginates SearchProducts).
func (s *ServiceCatalogService) ListCatalogProducts(ctx context.Context) ([]Product, error) {
	if s == nil || s.api == nil {
		return nil, fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var products []Product
	var pageToken *string
	for {
		input := &servicecatalog.SearchProductsInput{
			PageToken: pageToken,
		}
		output, err := s.api.SearchProducts(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("list catalog products: %w", err)
		}
		if output == nil {
			break
		}
		for _, pv := range output.ProductViewSummaries {
			products = append(products, Product{
				ProductID:          aws.ToString(pv.ProductId),
				ProductName:        aws.ToString(pv.Name),
				ProductType:        string(pv.Type),
				Owner:              aws.ToString(pv.Owner),
				ShortDescription:   aws.ToString(pv.ShortDescription),
				Distributor:        aws.ToString(pv.Distributor),
				SupportDescription: aws.ToString(pv.SupportDescription),
				HasDefaultPath:     pv.HasDefaultPath,
			})
		}
		pageToken = output.NextPageToken
		if pageToken == nil || *pageToken == "" {
			break
		}
	}
	return products, nil
}

// ListProvisioningArtifacts returns provisioning artifacts (versions) for a product accessible to the current user.
func (s *ServiceCatalogService) ListProvisioningArtifacts(ctx context.Context, productID string) ([]ProvisioningArtifact, error) {
	if s == nil || s.api == nil {
		return nil, fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	input := &servicecatalog.DescribeProductInput{
		Id: aws.String(productID),
	}
	output, err := s.api.DescribeProduct(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("describe product: %w", err)
	}
	if output == nil || len(output.ProvisioningArtifacts) == 0 {
		return []ProvisioningArtifact{}, nil
	}

	artifacts := make([]ProvisioningArtifact, 0, len(output.ProvisioningArtifacts))
	for _, d := range output.ProvisioningArtifacts {
		a := ProvisioningArtifact{
			ID:          aws.ToString(d.Id),
			Name:        aws.ToString(d.Name),
			Description: aws.ToString(d.Description),
			Guidance:    string(d.Guidance),
		}
		if d.CreatedTime != nil {
			a.CreatedTime = *d.CreatedTime
		}
		artifacts = append(artifacts, a)
	}
	return artifacts, nil
}

// DescribeProvisioningParameters returns parameters needed to provision a product (uses first launch path).
func (s *ServiceCatalogService) DescribeProvisioningParameters(ctx context.Context, productID, artifactID string) ([]ProvisioningParameter, error) {
	if s == nil || s.api == nil {
		return nil, fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	pathInput := &servicecatalog.ListLaunchPathsInput{
		ProductId: aws.String(productID),
	}
	pathOutput, err := s.api.ListLaunchPaths(ctx, pathInput)
	if err != nil {
		return nil, fmt.Errorf("list launch paths: %w", err)
	}
	if pathOutput == nil || len(pathOutput.LaunchPathSummaries) == 0 {
		return nil, fmt.Errorf("no launch paths available for this product")
	}

	pathID := pathOutput.LaunchPathSummaries[0].Id
	paramInput := &servicecatalog.DescribeProvisioningParametersInput{
		ProductId:              aws.String(productID),
		ProvisioningArtifactId: aws.String(artifactID),
		PathId:                 pathID,
	}
	paramOutput, err := s.api.DescribeProvisioningParameters(ctx, paramInput)
	if err != nil {
		return nil, fmt.Errorf("describe provisioning parameters: %w", err)
	}
	if paramOutput == nil {
		return []ProvisioningParameter{}, nil
	}

	params := make([]ProvisioningParameter, 0, len(paramOutput.ProvisioningArtifactParameters))
	for _, p := range paramOutput.ProvisioningArtifactParameters {
		param := ProvisioningParameter{
			Key:                   aws.ToString(p.ParameterKey),
			DefaultValue:          aws.ToString(p.DefaultValue),
			Description:           aws.ToString(p.Description),
			IsNoEcho:              p.IsNoEcho,
			ParameterType:         aws.ToString(p.ParameterType),
			ConstraintDescription: aws.ToString(p.ParameterConstraints.ConstraintDescription),
			Required:              p.DefaultValue == nil,
		}
		if p.ParameterConstraints != nil {
			param.AllowedValues = p.ParameterConstraints.AllowedValues
		}

		params = append(params, param)
	}
	return params, nil
}

// ProvisionProduct provisions a new product and returns the record ID.
func (s *ServiceCatalogService) ProvisionProduct(ctx context.Context, productID, artifactID, provisionedProductName string, parameters map[string]string) (string, error) {
	if s == nil || s.api == nil {
		return "", fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var provisioningParams []types.ProvisioningParameter
	for k, v := range parameters {
		provisioningParams = append(provisioningParams, types.ProvisioningParameter{
			Key:   aws.String(k),
			Value: aws.String(v),
		})
	}
	token := fmt.Sprintf("provision-%d", time.Now().UnixNano())
	input := &servicecatalog.ProvisionProductInput{
		ProductId:              aws.String(productID),
		ProvisioningArtifactId: aws.String(artifactID),
		ProvisionedProductName: aws.String(provisionedProductName),
		ProvisioningParameters: provisioningParams,
		ProvisionToken:         aws.String(token),
	}
	output, err := s.api.ProvisionProduct(ctx, input)
	if err != nil {
		return "", fmt.Errorf("provision product: %w", err)
	}
	if output == nil || output.RecordDetail == nil {
		return "", fmt.Errorf("provision product: empty response")
	}
	return aws.ToString(output.RecordDetail.RecordId), nil
}

const cloudFormationStackARNPrefix = "arn:aws:cloudformation:"

// getStackARNFromRecord returns the CloudFormation stack ARN from a provisioned product record's outputs.
func (s *ServiceCatalogService) getStackARNFromRecord(ctx context.Context, recordID string) (string, error) {
	if recordID == "" {
		return "", fmt.Errorf("record ID is required")
	}
	input := &servicecatalog.DescribeRecordInput{
		Id: aws.String(recordID),
	}
	output, err := s.api.DescribeRecord(ctx, input)
	if err != nil {
		return "", fmt.Errorf("describe record: %w", err)
	}
	if output == nil || len(output.RecordOutputs) == 0 {
		return "", fmt.Errorf("no record outputs found for record %s", recordID)
	}
	for _, out := range output.RecordOutputs {
		val := aws.ToString(out.OutputValue)
		if strings.HasPrefix(val, cloudFormationStackARNPrefix) {
			return val, nil
		}
	}
	return "", fmt.Errorf("no CloudFormation stack ARN in record outputs for record %s", recordID)
}

const ec2InstanceResourceType = "AWS::EC2::Instance"

// InstanceIDsForProvisionedProduct returns EC2 instance IDs for a provisioned product using its last successful record (CloudFormation stack).
// Use this for SSM shell or port-forward when the product backs onto EC2 instances.
func (s *ServiceCatalogService) InstanceIDsForProvisionedProduct(ctx context.Context, lastSuccessfulRecordID string) ([]string, error) {
	if s == nil || s.cfn == nil {
		return nil, fmt.Errorf("Service Catalog or CloudFormation not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stackARN, err := s.getStackARNFromRecord(ctx, lastSuccessfulRecordID)
	if err != nil {
		return nil, err
	}
	return s.cfn.ListStackResourcePhysicalIDs(ctx, stackARN, ec2InstanceResourceType)
}

// StartProvisionedProduct starts the EC2 instances underlying the provisioned product. It does not terminate anything.
func (s *ServiceCatalogService) StartProvisionedProduct(ctx context.Context, _, lastSuccessfulRecordID string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("Service Catalog client is not initialized")
	}
	if s.ec2 == nil || s.cfn == nil {
		return fmt.Errorf("EC2 or CloudFormation not configured for start/stop")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stackARN, err := s.getStackARNFromRecord(ctx, lastSuccessfulRecordID)
	if err != nil {
		return err
	}
	instanceIDs, err := s.cfn.ListStackResourcePhysicalIDs(ctx, stackARN, ec2InstanceResourceType)
	if err != nil {
		return err
	}
	if len(instanceIDs) == 0 {
		return fmt.Errorf("no EC2 instances found for this product")
	}
	return s.ec2.StartInstances(ctx, instanceIDs)
}

// StopProvisionedProduct stops the EC2 instances underlying the provisioned product. It does not terminate anything.
func (s *ServiceCatalogService) StopProvisionedProduct(ctx context.Context, _, lastSuccessfulRecordID string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("Service Catalog client is not initialized")
	}
	if s.ec2 == nil || s.cfn == nil {
		return fmt.Errorf("EC2 or CloudFormation not configured for start/stop")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	stackARN, err := s.getStackARNFromRecord(ctx, lastSuccessfulRecordID)
	if err != nil {
		return err
	}
	instanceIDs, err := s.cfn.ListStackResourcePhysicalIDs(ctx, stackARN, ec2InstanceResourceType)
	if err != nil {
		return err
	}
	if len(instanceIDs) == 0 {
		return fmt.Errorf("no EC2 instances found for this product")
	}
	return s.ec2.StopInstances(ctx, instanceIDs)
}

// TerminateProvisionedProduct deprovisions a provisioned product (removes it from Service Catalog and deletes the underlying stack/resources).
func (s *ServiceCatalogService) TerminateProvisionedProduct(ctx context.Context, provisionedProductID string) error {
	if s == nil || s.api == nil {
		return fmt.Errorf("Service Catalog client is not initialized")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if provisionedProductID == "" {
		return fmt.Errorf("provisioned product ID is required")
	}
	token := fmt.Sprintf("terminate-%d", time.Now().UnixNano())
	input := &servicecatalog.TerminateProvisionedProductInput{
		ProvisionedProductId: aws.String(provisionedProductID),
		TerminateToken:       aws.String(token),
	}
	_, err := s.api.TerminateProvisionedProduct(ctx, input)
	if err != nil {
		return fmt.Errorf("terminate provisioned product: %w", err)
	}
	return nil
}
