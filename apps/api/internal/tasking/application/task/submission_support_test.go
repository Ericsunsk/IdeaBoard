package task

import (
	"context"
	"strconv"
	"strings"

	"ideaboard/api/internal/billing"
	creditsdomain "ideaboard/api/internal/credits/domain"
	creditsports "ideaboard/api/internal/credits/ports"
	"ideaboard/api/internal/prompts"
	taskbilling "ideaboard/api/internal/tasking/application/billing"
	tasks "ideaboard/api/internal/tasking/domain"
	taskmedia "ideaboard/api/internal/tasking/ports/media"
	"ideaboard/api/internal/workflow"

	"github.com/jackc/pgx/v5"
)

type submitTestServiceDeps struct {
	SubmissionStore TaskSubmissionStore
	MediaValidator  taskmedia.Validator
	MediaPreparer   taskmedia.GenerationMediaPreparer
	PromptStore     FeaturePromptStore
	TaskBilling     BillingReadiness
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
			Prompts: promptStore, Media: deps.MediaValidator,
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

// The recording fakes below moved verbatim from the deleted legacy
// postgres-backed service_test.go (#816 Phase 3b-07): the surviving
// intake/validation tests still exercise them without any database.

type testSubmitMediaValidator struct{}

func (testSubmitMediaValidator) ValidateSubmitMediaRef(context.Context, int64, string) error {
	return nil
}

type recordingGenerationMediaPreparer struct {
	calls int
}

func (p *recordingGenerationMediaPreparer) PrepareGenerationMedia(_ context.Context, _ int64, _ string, snapshot *tasks.GenerationRequestSnapshot) (tasks.GenerationRequestSnapshot, error) {
	p.calls++
	prepared := *snapshot
	prepared.Media = append([]tasks.GenerationMediaIntent(nil), snapshot.Media...)
	for index := range prepared.Media {
		prepared.Media[index].AssetID = int64(index + 1)
		prepared.Media[index].Source = ""
	}
	return prepared, nil
}

type recordingTaskInputManifestBuilder struct{}

func (recordingTaskInputManifestBuilder) BuildTaskInputManifest(_ context.Context, _ int64, media *tasks.MediaInputs) (*tasks.TaskInputManifest, error) {
	if media == nil {
		return tasks.NewTaskInputManifest(nil, nil, nil, nil, nil)
	}
	ref := func(value tasks.MediaRef, suffix string) tasks.TaskInputManifestRef {
		return tasks.TaskInputManifestRef{AssetID: "asset_" + suffix, DerivativeID: "derivative_" + suffix, DerivativeTier: tasks.TierProcessingInput, StorageProfileID: "test", ObjectKey: "uploads/" + suffix + ".webp", ObjectGeneration: 1, ChecksumSHA256: strings.Repeat("a", 64), MIMEType: "image/webp", SizeBytes: 1, Role: value.Role, Weight: value.Weight}
	}
	inputs := make([]tasks.TaskInputManifestRef, 0, len(media.Inputs))
	for index, value := range media.Inputs {
		inputs = append(inputs, ref(value, strconv.Itoa(index+1)))
	}
	references := make([]tasks.TaskInputManifestRef, 0, len(media.References))
	for index, value := range media.References {
		references = append(references, ref(value, strconv.Itoa(100+index)))
	}
	var original, mask *tasks.TaskInputManifestRef
	if media.Original != nil {
		value := ref(*media.Original, "original")
		original = &value
	}
	if media.Mask != nil {
		value := ref(*media.Mask, "mask")
		mask = &value
	}
	return tasks.NewTaskInputManifest(inputs, references, original, mask, media.UseMask)
}

type fakeFeaturePromptStore struct {
	configs map[string]prompts.FeaturePromptRuntimeConfig
	err     error
}

func (s fakeFeaturePromptStore) GetRuntimeConfig(_ context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error) {
	if s.err != nil {
		return prompts.FeaturePromptRuntimeConfig{}, s.err
	}
	if s.configs == nil {
		return prompts.FeaturePromptRuntimeConfig{UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug)}, nil
	}
	config, ok := s.configs[slug]
	if !ok {
		return prompts.FeaturePromptRuntimeConfig{UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(slug)}, nil
	}
	return config, nil
}

type recordingSubmissionStore struct {
	calls  []submitCall
	result tasks.TaskSubmissionResult
	err    error
}

type submitCall struct {
	canonical CanonicalTaskRequest
}

func (s *recordingSubmissionStore) SubmitTask(_ context.Context, canonical CanonicalTaskRequest) (tasks.TaskSubmissionResult, error) {
	if s.err != nil {
		return tasks.TaskSubmissionResult{}, s.err
	}
	s.calls = append(s.calls, submitCall{canonical: canonical})
	if s.result.TaskID != "" {
		return s.result, nil
	}
	return tasks.TaskSubmissionResult{TaskID: "task_recorded_" + strconv.Itoa(len(s.calls)), Created: true}, nil
}
