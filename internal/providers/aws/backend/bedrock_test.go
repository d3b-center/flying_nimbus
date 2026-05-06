package aws

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/bedrock"
	bedrocktypes "github.com/aws/aws-sdk-go-v2/service/bedrock/types"
)

// ── mock ──────────────────────────────────────────────────────────────────────

type mockBedrockAPI struct {
	fmOut  *bedrock.ListFoundationModelsOutput
	fmErr  error
	ipOuts []*bedrock.ListInferenceProfilesOutput
	ipErr  error
	ipCall int
}

func (m *mockBedrockAPI) ListFoundationModels(
	_ context.Context,
	_ *bedrock.ListFoundationModelsInput,
	_ ...func(*bedrock.Options),
) (*bedrock.ListFoundationModelsOutput, error) {
	if m.fmErr != nil {
		return nil, m.fmErr
	}
	if m.fmOut == nil {
		return &bedrock.ListFoundationModelsOutput{}, nil
	}
	return m.fmOut, nil
}

func (m *mockBedrockAPI) ListInferenceProfiles(
	_ context.Context,
	_ *bedrock.ListInferenceProfilesInput,
	_ ...func(*bedrock.Options),
) (*bedrock.ListInferenceProfilesOutput, error) {
	if m.ipErr != nil {
		return nil, m.ipErr
	}
	if len(m.ipOuts) == 0 {
		return &bedrock.ListInferenceProfilesOutput{}, nil
	}
	out := m.ipOuts[m.ipCall]
	m.ipCall++
	return out, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func ptr[T any](v T) *T { return &v }

func staticCfg(region string) aws.Config {
	return aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider("AKID", "SECRET", "TOKEN"),
	}
}

func newSvc(api bedrockAPI) *BedrockService {
	return &BedrockService{api: api, cfg: staticCfg("us-east-1")}
}

// ── BedrockModel list.Item interface ─────────────────────────────────────────

func TestBedrockModel_ListItemInterface(t *testing.T) {
	m := BedrockModel{
		ModelID:      "anthropic.claude-3-sonnet-20240229-v1:0",
		ModelName:    "Claude 3 Sonnet",
		ProviderName: "Anthropic",
	}

	if m.Title() != "Claude 3 Sonnet" {
		t.Errorf("Title() = %q", m.Title())
	}
	if m.Description() != "Anthropic · anthropic.claude-3-sonnet-20240229-v1:0" {
		t.Errorf("Description() = %q", m.Description())
	}
	fv := m.FilterValue()
	if !strings.Contains(fv, "Claude 3 Sonnet") ||
		!strings.Contains(fv, "Anthropic") ||
		!strings.Contains(fv, "anthropic.claude-3-sonnet-20240229-v1:0") {
		t.Errorf("FilterValue() = %q, expected all three fields present", fv)
	}
}

// ── BedrockToken list.Item interface ─────────────────────────────────────────

func TestBedrockToken_ListItemInterface(t *testing.T) {
	expiry := time.Date(2026, 1, 2, 15, 4, 5, 0, time.UTC)
	tok := BedrockToken{
		Token:  "bedrock-api-key-abc123",
		Expiry: expiry,
		Region: "us-east-1",
	}

	if tok.Title() != "Bedrock API Token" {
		t.Errorf("Title() = %q", tok.Title())
	}
	if !strings.Contains(tok.Description(), "us-east-1") {
		t.Errorf("Description() missing region: %q", tok.Description())
	}
	if !strings.Contains(tok.Description(), expiry.Format(time.RFC3339)) {
		t.Errorf("Description() missing expiry: %q", tok.Description())
	}
	if tok.FilterValue() != "Bedrock API Token" {
		t.Errorf("FilterValue() = %q", tok.FilterValue())
	}
}

// ── IsBedrockEnabled ──────────────────────────────────────────────────────────

func TestIsBedrockEnabled_ReturnsTrue_WhenAPISucceeds(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{fmOut: &bedrock.ListFoundationModelsOutput{}})
	if !svc.IsBedrockEnabled(context.Background()) {
		t.Error("expected true, got false")
	}
}

func TestIsBedrockEnabled_ReturnsFalse_WhenAPIFails(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{fmErr: errors.New("access denied")})
	if svc.IsBedrockEnabled(context.Background()) {
		t.Error("expected false, got true")
	}
}

// ── GenerateShortTermToken ────────────────────────────────────────────────────

func TestGenerateShortTermToken_TokenFormat(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{})

	tok, err := svc.GenerateShortTermToken(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.HasPrefix(tok.Token, bedrockTokenPrefix) {
		t.Errorf("token missing prefix %q: %q", bedrockTokenPrefix, tok.Token)
	}

	encoded := strings.TrimPrefix(tok.Token, bedrockTokenPrefix)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("token suffix is not valid base64: %v", err)
	}

	payload := string(decoded)
	if !strings.Contains(payload, bedrockDefaultHost) {
		t.Errorf("decoded payload missing host %q: %s", bedrockDefaultHost, payload)
	}
	if !strings.HasSuffix(payload, bedrockTokenVersion) {
		t.Errorf("decoded payload missing version suffix: %s", payload)
	}
}

func TestGenerateShortTermToken_SetsRegionAndExpiry(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{})

	before := time.Now()
	tok, err := svc.GenerateShortTermToken(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if tok.Region != "us-east-1" {
		t.Errorf("Region = %q, want us-east-1", tok.Region)
	}
	if tok.Expiry.Before(before.Add(55 * time.Minute)) {
		t.Errorf("Expiry %v seems too soon", tok.Expiry)
	}
}

func TestGenerateShortTermToken_ClampsExpiryToMax(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{})

	for _, dur := range []time.Duration{0, -1 * time.Hour, 24 * time.Hour} {
		tok, err := svc.GenerateShortTermToken(context.Background(), dur)
		if err != nil {
			t.Fatalf("dur=%v unexpected error: %v", dur, err)
		}
		if tok.Expiry.Before(time.Now().Add(bedrockMaxExpiry - time.Minute)) {
			t.Errorf("dur=%v: expiry %v not clamped to max", dur, tok.Expiry)
		}
	}
}

func TestGenerateShortTermToken_FailsWithNoCredentials(t *testing.T) {
	svc := &BedrockService{
		api: &mockBedrockAPI{},
		cfg: aws.Config{
			Region: "us-east-1",
			Credentials: aws.CredentialsProviderFunc(func(_ context.Context) (aws.Credentials, error) {
				return aws.Credentials{}, errors.New("no credentials configured")
			}),
		},
	}
	_, err := svc.GenerateShortTermToken(context.Background(), time.Hour)
	if err == nil {
		t.Fatal("expected error with failing credentials, got nil")
	}
}

// ── ListAvailableModels — foundation model filters ────────────────────────────

func TestListAvailableModels_Empty(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

func TestListAvailableModels_FoundationModelsAPIError(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{fmErr: errors.New("throttled")})
	_, err := svc.ListAvailableModels(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListAvailableModels_SkipsNilIDOrName(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{ModelId: nil, ModelName: ptr("Name")},
				{ModelId: ptr("id"), ModelName: nil},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d", len(models))
	}
}

// Models with empty OutputModalities must be included — this is the Claude pattern.
func TestListAvailableModels_IncludesModel_WhenOutputModalitiesEmpty(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:          ptr("anthropic.claude-3-sonnet"),
					ModelName:        ptr("Claude 3 Sonnet"),
					ProviderName:     ptr("Anthropic"),
					OutputModalities: nil,
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ModelID != "anthropic.claude-3-sonnet" {
		t.Errorf("unexpected model ID: %q", models[0].ModelID)
	}
}

func TestListAvailableModels_IncludesModel_WithTextOutput(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:          ptr("amazon.nova-pro"),
					ModelName:        ptr("Amazon Nova Pro"),
					ProviderName:     ptr("Amazon"),
					OutputModalities: []bedrocktypes.ModelModality{bedrocktypes.ModelModalityText},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
}

// Models whose OutputModalities is explicitly set to IMAGE only must be excluded.
func TestListAvailableModels_ExcludesModel_WithImageOnlyOutput(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:          ptr("stability.sd3-large-v1:0"),
					ModelName:        ptr("Stable Diffusion 3 Large"),
					ProviderName:     ptr("Stability AI"),
					OutputModalities: []bedrocktypes.ModelModality{bedrocktypes.ModelModalityImage},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d — image-only model should be excluded", len(models))
	}
}

func TestListAvailableModels_ExcludesLegacyModel(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:      ptr("anthropic.claude-v2"),
					ModelName:    ptr("Claude 2"),
					ProviderName: ptr("Anthropic"),
					ModelLifecycle: &bedrocktypes.FoundationModelLifecycle{
						Status: bedrocktypes.FoundationModelLifecycleStatusLegacy,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d — legacy model should be excluded", len(models))
	}
}

func TestListAvailableModels_IncludesActiveModel(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:      ptr("amazon.nova-lite"),
					ModelName:    ptr("Amazon Nova Lite"),
					ProviderName: ptr("Amazon"),
					ModelLifecycle: &bedrocktypes.FoundationModelLifecycle{
						Status: bedrocktypes.FoundationModelLifecycleStatusActive,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
}

// Models with empty InferenceTypesSupported must be included — this is the Claude pattern.
func TestListAvailableModels_IncludesModel_WhenInferenceTypesEmpty(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:                 ptr("anthropic.claude-3-haiku"),
					ModelName:               ptr("Claude 3 Haiku"),
					ProviderName:            ptr("Anthropic"),
					InferenceTypesSupported: nil,
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
}

func TestListAvailableModels_IncludesModel_WithOnDemandInference(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:                 ptr("meta.llama3-70b"),
					ModelName:               ptr("Llama 3 70B"),
					ProviderName:            ptr("Meta"),
					InferenceTypesSupported: []bedrocktypes.InferenceType{bedrocktypes.InferenceTypeOnDemand},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
}

// Models with only PROVISIONED in InferenceTypesSupported must be excluded.
func TestListAvailableModels_ExcludesModel_ProvisionedOnly(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{
					ModelId:                 ptr("some.provisioned-only-model"),
					ModelName:               ptr("Provisioned Only"),
					ProviderName:            ptr("Some"),
					InferenceTypesSupported: []bedrocktypes.InferenceType{bedrocktypes.InferenceTypeProvisioned},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d — provisioned-only model should be excluded", len(models))
	}
}

// ── ListAvailableModels — inference profiles ──────────────────────────────────

func TestListAvailableModels_IncludesActiveInferenceProfile(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		ipOuts: []*bedrock.ListInferenceProfilesOutput{
			{
				InferenceProfileSummaries: []bedrocktypes.InferenceProfileSummary{
					{
						InferenceProfileId:   ptr("us.anthropic.claude-3-5-sonnet-20241022-v2:0"),
						InferenceProfileName: ptr("Claude 3.5 Sonnet v2"),
						Status:               bedrocktypes.InferenceProfileStatusActive,
						Type:                 bedrocktypes.InferenceProfileTypeSystemDefined,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ModelID != "us.anthropic.claude-3-5-sonnet-20241022-v2:0" {
		t.Errorf("unexpected model ID: %q", models[0].ModelID)
	}
	if models[0].ProviderName != "Anthropic" {
		t.Errorf("unexpected provider: %q", models[0].ProviderName)
	}
}

func TestListAvailableModels_ExcludesInactiveInferenceProfile(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		ipOuts: []*bedrock.ListInferenceProfilesOutput{
			{
				InferenceProfileSummaries: []bedrocktypes.InferenceProfileSummary{
					{
						InferenceProfileId:   ptr("us.anthropic.claude-inactive"),
						InferenceProfileName: ptr("Claude Inactive"),
						Status:               bedrocktypes.InferenceProfileStatus("INACTIVE"),
						Type:                 bedrocktypes.InferenceProfileTypeSystemDefined,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Errorf("expected 0 models, got %d — inactive profile should be excluded", len(models))
	}
}

// A model appearing in both ListFoundationModels and ListInferenceProfiles
// must appear exactly once in the result.
func TestListAvailableModels_DeduplicatesAcrossSources(t *testing.T) {
	const sharedID = "anthropic.claude-3-sonnet"
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{ModelId: ptr(sharedID), ModelName: ptr("Claude 3 Sonnet"), ProviderName: ptr("Anthropic")},
			},
		},
		ipOuts: []*bedrock.ListInferenceProfilesOutput{
			{
				InferenceProfileSummaries: []bedrocktypes.InferenceProfileSummary{
					{
						InferenceProfileId:   ptr(sharedID),
						InferenceProfileName: ptr("Claude 3 Sonnet"),
						Status:               bedrocktypes.InferenceProfileStatusActive,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("expected 1 model after deduplication, got %d", len(models))
	}
}

// An error from ListInferenceProfiles must be non-fatal: foundation models
// already collected should still be returned.
func TestListAvailableModels_InferenceProfilesError_IsNonFatal(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		fmOut: &bedrock.ListFoundationModelsOutput{
			ModelSummaries: []bedrocktypes.FoundationModelSummary{
				{ModelId: ptr("amazon.nova-pro"), ModelName: ptr("Amazon Nova Pro"), ProviderName: ptr("Amazon")},
			},
		},
		ipErr: errors.New("profiles API unavailable"),
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(models) != 1 {
		t.Errorf("expected 1 foundation model even when profiles API fails, got %d", len(models))
	}
}

func TestListAvailableModels_PaginatesInferenceProfiles(t *testing.T) {
	svc := newSvc(&mockBedrockAPI{
		ipOuts: []*bedrock.ListInferenceProfilesOutput{
			{
				InferenceProfileSummaries: []bedrocktypes.InferenceProfileSummary{
					{
						InferenceProfileId:   ptr("us.anthropic.claude-page1"),
						InferenceProfileName: ptr("Claude Page 1"),
						Status:               bedrocktypes.InferenceProfileStatusActive,
					},
				},
				NextToken: ptr("page2-token"),
			},
			{
				InferenceProfileSummaries: []bedrocktypes.InferenceProfileSummary{
					{
						InferenceProfileId:   ptr("us.anthropic.claude-page2"),
						InferenceProfileName: ptr("Claude Page 2"),
						Status:               bedrocktypes.InferenceProfileStatusActive,
					},
				},
			},
		},
	})
	models, err := svc.ListAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("expected 2 models across 2 pages, got %d", len(models))
	}
}

// ── inferProviderFromProfileID ────────────────────────────────────────────────

func TestInferProviderFromProfileID(t *testing.T) {
	tests := []struct {
		id   string
		want string
	}{
		{"us.anthropic.claude-3-5-sonnet-20241022-v2:0", "Anthropic"},
		{"eu.anthropic.claude-3-haiku-20240307-v1:0", "Anthropic"},
		{"us.amazon.nova-pro-v1:0", "Amazon"},
		{"us.meta.llama3-70b-instruct-v1:0", "Meta"},
		{"eu.mistral.mistral-large-2402-v1:0", "Mistral AI"},
		{"us.cohere.command-r-v1:0", "Cohere"},
		{"us.ai21.jamba-1-5-large-v1:0", "AI21 Labs"},
		{"us.stability.stable-diffusion-xl-v1:0", "Stability AI"},
		{"us.newcorp.some-model-v1:0", "newcorp"},
		{"nodots", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := inferProviderFromProfileID(tt.id)
			if got != tt.want {
				t.Errorf("inferProviderFromProfileID(%q) = %q, want %q", tt.id, got, tt.want)
			}
		})
	}
}
