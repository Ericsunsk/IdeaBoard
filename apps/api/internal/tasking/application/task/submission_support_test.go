package task

import (
	"context"

	"ideaboard/api/internal/billing"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/prompts"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

type submitTestServiceDeps struct {
	ModelStore      ModelStore
	SubmissionStore TaskSubmissionStore
	MediaValidator  taskmedia.Validator
	MediaPreparer   taskmedia.GenerationMediaPreparer
	PromptStore     FeaturePromptStore
	TaskBilling     BillingReadiness
}

type fakeIntakeModelStore struct {
	model string
	err   error
}

func (s *fakeIntakeModelStore) ResolveModel(context.Context, string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	if s.model != "" {
		return s.model, nil
	}
	return "openai/gpt-image-1", nil
}

func modelStoreOrDefault(store ModelStore) ModelStore {
	if store != nil {
		return store
	}
	return &fakeIntakeModelStore{}
}

func newSubmitServiceForTest(deps submitTestServiceDeps, opts ServiceOptions) *Service {
	promptStore := deps.PromptStore
	if promptStore == nil {
		configs := make(map[string]prompts.FeaturePromptRuntimeConfig)
		for _, slug := range workflow.SupportedFeatureSlugs() {
			configs[slug] = prompts.FeaturePromptRuntimeConfig{
				Prompt: "{{user_prompt}}", UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug),
			}
		}
		promptStore = fakeFeaturePromptStore{configs: configs}
	}
	mediaPreparer := deps.MediaPreparer
	if mediaPreparer == nil {
		mediaPreparer = &recordingGenerationMediaPreparer{}
	}
	return NewService(ServiceDeps{
		TaskIntake: NewTaskIntake(TaskIntakeDeps{
			Models: modelStoreOrDefault(deps.ModelStore), Prompts: promptStore, Media: deps.MediaValidator,
			MediaPreparer: mediaPreparer, MediaManifest: recordingTaskInputManifestBuilder{},
		}),
		SubmissionStore: deps.SubmissionStore,
		TaskBilling:     deps.TaskBilling,
	}, opts)
}

type handlerBillingQuoter struct{ err error }

func (q handlerBillingQuoter) QuoteAtResolution(ctx context.Context, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	if q.err != nil {
		return billing.Quote{}, q.err
	}
	sku, err := billing.CanonicalizeSKU(rawSKU)
	if err != nil {
		return billing.Quote{}, err
	}
	return billing.Quote{PriceSnapshot: billing.PriceSnapshot{SKU: sku, DisplayName: "Test AI image", PriceVersionID: 1, PriceVersion: 1, OutputResolution: resolution, UnitCredits: 3}, Quantity: quantity, TotalCredits: 3 * quantity}, nil
}

func (q handlerBillingQuoter) QuoteAtResolutionInTx(ctx context.Context, _ pgx.Tx, rawSKU string, resolution billing.OutputResolution, quantity int64) (billing.Quote, error) {
	return q.QuoteAtResolution(ctx, rawSKU, resolution, quantity)
}

func newHandlerTaskBillingService() *taskbilling.Service {
	return taskbilling.NewService(handlerBillingQuoter{}, handlerBillingCredits{})
}

type handlerBillingCredits struct{}

func (handlerBillingCredits) GetWallet(context.Context, int64) (creditsdomain.WalletDetails, error) {
	return creditsdomain.WalletDetails{WalletBalance: creditsdomain.WalletBalance{Available: 100}, Status: creditsdomain.WalletStatusActive}, nil
}

func (handlerBillingCredits) ListLedger(context.Context, creditsdomain.ListLedgerInput) (creditsdomain.LedgerPage, error) {
	return creditsdomain.LedgerPage{}, nil
}

func (handlerBillingCredits) InTx(pgx.Tx) creditsports.Commands { return nil }
