package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

const (
	bedrockTokenPrefix  = "bedrock-api-key-"
	bedrockTokenVersion = "&Version=1"
	bedrockDefaultURL   = "https://bedrock.amazonaws.com/"
	bedrockDefaultHost  = "bedrock.amazonaws.com"
	bedrockServiceName  = "bedrock"
	bedrockMaxExpiry    = 12 * time.Hour
	// emptyPayloadHash is the hex-encoded SHA-256 hash of an empty string.
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

// BedrockModel represents a Bedrock foundation model available for text generation.
type BedrockModel struct {
	ModelID      string
	ModelName    string
	ProviderName string
}

// Title returns the model name for list display.
func (m BedrockModel) Title() string { return m.ModelName }

// Description returns the provider and model ID for list display.
func (m BedrockModel) Description() string { return m.ProviderName + " · " + m.ModelID }

// FilterValue returns searchable text for the model.
func (m BedrockModel) FilterValue() string {
	return m.ModelName + " " + m.ProviderName + " " + m.ModelID
}

// BedrockToken holds a short-term Bedrock API token and its metadata.
type BedrockToken struct {
	// Token is the bearer token string (bedrock-api-key-...).
	Token  string
	Expiry time.Time
	Region string
}

// Title returns the item title for list display.
func (t BedrockToken) Title() string { return "Bedrock API Token" }

// Description returns the region/expiry info for list display.
func (t BedrockToken) Description() string {
	return fmt.Sprintf("%s — expires %s", t.Region, t.Expiry.Format(time.RFC3339))
}

// FilterValue returns the token title for list filtering.
func (t BedrockToken) FilterValue() string { return "Bedrock API Token" }

type bedrockAPI interface {
	ListFoundationModels(ctx context.Context, params *bedrock.ListFoundationModelsInput, optFns ...func(*bedrock.Options)) (*bedrock.ListFoundationModelsOutput, error)
	ListInferenceProfiles(ctx context.Context, params *bedrock.ListInferenceProfilesInput, optFns ...func(*bedrock.Options)) (*bedrock.ListInferenceProfilesOutput, error)
}

// BedrockService checks Bedrock availability and generates short-term API tokens.
type BedrockService struct {
	api bedrockAPI
	cfg aws.Config
}

// InitBedrockService creates a new Bedrock service client.
func InitBedrockService(cfg aws.Config) *BedrockService {
	slog.Debug("Initializing Bedrock Service")
	client := bedrock.NewFromConfig(cfg)
	return &BedrockService{api: client, cfg: cfg}
}

// IsBedrockEnabled checks whether Bedrock is accessible in the current account and region
// by calling ListFoundationModels as a lightweight availability probe.
func (s BedrockService) IsBedrockEnabled(ctx context.Context) bool {
	_, err := s.api.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		slog.Warn(fmt.Sprintf("Bedrock not accessible: %v", err))
		return false
	}
	return true
}

// GenerateShortTermToken creates a short-term Bedrock API bearer token valid for up to 12 hours.
//
// The token format matches the official aws-bedrock-token-generator libraries:
//
//	bedrock-api-key-<base64(presigned_url_without_https_prefix + "&Version=1")>
//
// Internally it builds a SigV4 presigned POST to https://bedrock.amazonaws.com/
// with Action=CallWithBearerToken, then base64-encodes the result.
func (s BedrockService) GenerateShortTermToken(ctx context.Context, expiresIn time.Duration) (*BedrockToken, error) {
	if expiresIn <= 0 || expiresIn > bedrockMaxExpiry {
		expiresIn = bedrockMaxExpiry
	}

	creds, err := s.cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
	}

	u, err := url.Parse(bedrockDefaultURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bedrock URL: %w", err)
	}

	q := u.Query()
	q.Set("Action", "CallWithBearerToken")
	q.Set("X-Amz-Expires", strconv.FormatInt(int64(expiresIn/time.Second), 10))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build presign request: %w", err)
	}
	req.Header.Set("host", bedrockDefaultHost)

	signer := v4.NewSigner()
	presignedURL, _, err := signer.PresignHTTP(
		ctx, creds, req,
		emptyPayloadHash,
		bedrockServiceName, s.cfg.Region,
		time.Now(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to presign bedrock request: %w", err)
	}

	stripped := strings.TrimPrefix(presignedURL, "https://")
	withVersion := stripped + bedrockTokenVersion
	encoded := base64.StdEncoding.EncodeToString([]byte(withVersion))
	token := bedrockTokenPrefix + encoded

	return &BedrockToken{
		Token:  token,
		Expiry: time.Now().Add(expiresIn),
		Region: s.cfg.Region,
	}, nil
}

// ListAvailableModels returns all text-capable, non-legacy Bedrock models the
// caller can use. It merges two sources:
//
//  1. Foundation models from ListFoundationModels – base model IDs like
//     anthropic.claude-3-sonnet-20240229-v1:0
//
//  2. System-defined cross-region inference profiles from ListInferenceProfiles –
//     profile IDs like us.anthropic.claude-3-5-sonnet-20241022-v2:0, which is
//     where newer Claude (and other) models live exclusively.
//
// Models that are proven to produce no text output, or that are confirmed as
// provisioned-throughput-only, are excluded. Models whose API fields are empty
// or nil are kept to avoid false negatives (several Anthropic models omit
// OutputModalities and InferenceTypesSupported entirely).
func (s BedrockService) ListAvailableModels(ctx context.Context) ([]BedrockModel, error) {
	seen := make(map[string]struct{})
	var models []BedrockModel

	// ── 1. Foundation models ──────────────────────────────────────────────────
	fmOut, err := s.api.ListFoundationModels(ctx, &bedrock.ListFoundationModelsInput{})
	if err != nil {
		return nil, fmt.Errorf("list foundation models: %w", err)
	}

	for _, m := range fmOut.ModelSummaries {
		if m.ModelId == nil || m.ModelName == nil {
			continue
		}

		// Exclude only when OutputModalities is explicitly set and lacks TEXT.
		if len(m.OutputModalities) > 0 {
			hasText := false
			for _, mod := range m.OutputModalities {
				if mod == bedrocktypes.ModelModalityText {
					hasText = true
					break
				}
			}
			if !hasText {
				slog.Debug("bedrock: skipping non-text model", "id", *m.ModelId)
				continue
			}
		}

		// Exclude models confirmed to be in legacy lifecycle.
		if m.ModelLifecycle != nil &&
			m.ModelLifecycle.Status == bedrocktypes.FoundationModelLifecycleStatusLegacy {
			slog.Debug("bedrock: skipping legacy model", "id", *m.ModelId)
			continue
		}

		// Exclude only when InferenceTypesSupported is explicitly set and
		// contains no ON_DEMAND entry (i.e., provisioned-throughput only).
		if len(m.InferenceTypesSupported) > 0 {
			hasOnDemand := false
			for _, inf := range m.InferenceTypesSupported {
				if inf == bedrocktypes.InferenceTypeOnDemand {
					hasOnDemand = true
					break
				}
			}
			if !hasOnDemand {
				slog.Debug("bedrock: skipping provisioned-only model", "id", *m.ModelId)
				continue
			}
		}

		providerName := ""
		if m.ProviderName != nil {
			providerName = *m.ProviderName
		}

		seen[*m.ModelId] = struct{}{}
		models = append(models, BedrockModel{
			ModelID:      *m.ModelId,
			ModelName:    *m.ModelName,
			ProviderName: providerName,
		})
	}

	// ── 2. Cross-region inference profiles (SYSTEM_DEFINED) ──────────────────
	// Newer model generations (e.g. Claude 3.5 Sonnet, Claude 3.7, Claude 4)
	// are only reachable via these profile IDs and do not appear in
	// ListFoundationModels.
	var nextToken *string
	for {
		ipOut, err := s.api.ListInferenceProfiles(ctx, &bedrock.ListInferenceProfilesInput{
			TypeEquals: bedrocktypes.InferenceProfileTypeSystemDefined,
			NextToken:  nextToken,
		})
		if err != nil {
			slog.Warn("bedrock: could not list inference profiles", "err", err)
			break
		}

		for _, p := range ipOut.InferenceProfileSummaries {
			if p.InferenceProfileId == nil || p.InferenceProfileName == nil {
				continue
			}
			if p.Status != bedrocktypes.InferenceProfileStatusActive {
				continue
			}
			id := *p.InferenceProfileId
			if _, already := seen[id]; already {
				continue
			}
			seen[id] = struct{}{}
			models = append(models, BedrockModel{
				ModelID:      id,
				ModelName:    *p.InferenceProfileName,
				ProviderName: inferProviderFromProfileID(id),
			})
		}

		if ipOut.NextToken == nil {
			break
		}
		nextToken = ipOut.NextToken
	}

	slog.Debug(fmt.Sprintf("bedrock: %d models available", len(models)))
	return models, nil
}

// inferProviderFromProfileID derives a human-readable provider name from a
// cross-region inference profile ID such as
// "us.anthropic.claude-3-5-sonnet-20241022-v2:0".
func inferProviderFromProfileID(id string) string {
	parts := strings.SplitN(id, ".", 3)
	if len(parts) >= 2 {
		provider := parts[1]
		switch strings.ToLower(provider) {
		case "anthropic":
			return "Anthropic"
		case "amazon":
			return "Amazon"
		case "meta":
			return "Meta"
		case "mistral":
			return "Mistral AI"
		case "cohere":
			return "Cohere"
		case "ai21":
			return "AI21 Labs"
		case "stability":
			return "Stability AI"
		}
		return provider
	}
	return ""
}
