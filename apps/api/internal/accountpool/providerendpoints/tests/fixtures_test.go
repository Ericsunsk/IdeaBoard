package tests

import (
	"context"
	"database/sql"
	"testing"

	"ideaboard/api/internal/accountpool/providerendpoints/application"
	"ideaboard/api/internal/accountpool/providerendpoints/domain"
	"ideaboard/api/internal/accountpool/providerendpoints/infrastructure/postgres"
	endpointHttp "ideaboard/api/internal/accountpool/providerendpoints/interfaces/http"
	"ideaboard/api/internal/accountpool/providerendpoints/ports"
	"ideaboard/api/internal/platform/outbound"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/testdb"
)

type (
	Endpoint                    = domain.Endpoint
	ProtocolDefinition          = domain.ProtocolDefinition
	Repository                  = postgres.Repository
	Service                     = application.Service
	Handler                     = endpointHttp.Handler
	CreateEndpointRequest       = endpointHttp.CreateEndpointRequest
	UpdateEndpointRequest       = endpointHttp.UpdateEndpointRequest
	UpdateEndpointStatusRequest = endpointHttp.UpdateEndpointStatusRequest
	DetectModelsRequest         = endpointHttp.DetectModelsRequest
	detectedModelsResponse      = endpointHttp.DetectedModelsResponse
)

const (
	detectModelsTimeout   = application.DetectModelsTimeout
	maxDetectedModels     = application.MaxDetectedModels
	ProtocolModelAPI      = domain.ProtocolModelAPI
	ProtocolImagePipeline = domain.ProtocolImagePipeline
	ProtocolImageFill     = domain.ProtocolImageFill
	CapabilityGeneration  = domain.CapabilityGeneration
	CapabilityCutout      = domain.CapabilityCutout
	CapabilityUpscale     = domain.CapabilityUpscale
	CapabilityFill        = domain.CapabilityFill
)

var (
	ErrNameConflict         = domain.ErrNameConflict
	ErrURLConflict          = domain.ErrURLConflict
	ErrNotFound             = domain.ErrNotFound
	ErrInvalidStatus        = domain.ErrInvalidStatus
	ErrAPIKeyRequired       = domain.ErrAPIKeyRequired
	ErrUnsupportedProtocol  = domain.ErrUnsupportedProtocol
	ErrImageAPIRequiresFunc = domain.ErrImageAPIRequiresFunc
	ErrInvalidBaseURL       = domain.ErrInvalidBaseURL
	ErrUnsupportedAPIType   = domain.ErrUnsupportedAPIType

	NewRepository = postgres.NewRepository
	NewService    = application.NewService

	buildModelDetectionURL                         = application.BuildModelDetectionURL
	buildGoogleCloudModelProbeURL                  = application.BuildGoogleCloudModelProbeURL
	parseModelIDs                                  = application.ParseModelIDs
	providerModelDetectionTransportFailureCategory = application.ProviderModelDetectionTransportFailureCategory
	googleCloudImageModelCandidates                = application.GoogleCloudImageModelCandidates
	respondProviderEndpointConflict                = endpointHttp.RespondProviderEndpointConflict
)

func NewHandler(repo ports.Repository, appEnv ...string) *Handler {
	svc := application.NewService(repo, appEnv...)
	return endpointHttp.NewHandler(svc)
}

func newProviderEndpointRepositoryTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return testdb.Open(t)
}

func seedProviderEndpoint(t testing.TB, ctx context.Context, repo ports.Repository, endpoint *Endpoint) *Endpoint {
	t.Helper()
	if endpoint.Protocol == "" {
		endpoint.Protocol = ProtocolModelAPI
	}
	if err := repo.Create(ctx, endpoint); err != nil {
		t.Fatalf("seed provider endpoint failed: %v", err)
	}
	return endpoint
}

type testProviderEndpointOutboundPolicy struct{ hosts []string }

func (p testProviderEndpointOutboundPolicy) GetOutboundPolicy(context.Context) (settings.OutboundPolicySetting, error) {
	return settings.OutboundPolicySetting{AllowHosts: p.hosts}, nil
}

func setProviderEndpointTestAllowHosts(t *testing.T, hosts ...string) {
	t.Helper()
	restore := outbound.ConfigurePolicyProvider(testProviderEndpointOutboundPolicy{hosts: hosts})
	t.Cleanup(restore)
}
