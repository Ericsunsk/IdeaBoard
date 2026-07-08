package prompts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/workflow"
)

const promptEvalSuitePatternGenV1 = "pattern-gen-golden-v1"
const promptEvalLatestConfigKeyPrefix = "prompt_eval_report:latest:"
const promptEvalDraftConfigKeyPrefix = "prompt_eval_report:draft:"

var ErrPromptEvalUnsupported = errors.New("prompt eval unsupported for feature")

type PromptEvalGateError struct {
	Report *FeaturePromptEvalReport
}

func (e *PromptEvalGateError) Error() string {
	if e == nil || e.Report == nil || strings.TrimSpace(e.Report.Summary) == "" {
		return "prompt eval gate blocked publish"
	}
	return "prompt eval gate blocked publish: " + e.Report.Summary
}

type FeaturePromptEvalReport struct {
	ID             int64                            `json:"id"`
	FeatureSlug    string                           `json:"feature_slug"`
	DraftHash      string                           `json:"draft_hash"`
	SuiteID        string                           `json:"suite_id"`
	Passed         bool                             `json:"passed"`
	Summary        string                           `json:"summary"`
	FixtureResults []FeaturePromptEvalFixtureResult `json:"fixture_results"`
	CreatedBy      *int64                           `json:"created_by,omitempty"`
	CreatedAt      time.Time                        `json:"created_at"`
}

type FeaturePromptEvalFixtureResult struct {
	FixtureID         string   `json:"fixture_id"`
	Name              string   `json:"name"`
	Passed            bool     `json:"passed"`
	ExpectedContains  []string `json:"expected_contains"`
	MissingExpected   []string `json:"missing_expected"`
	ForbiddenContains []string `json:"forbidden_contains"`
	FoundForbidden    []string `json:"found_forbidden"`
	RenderedPrompt    string   `json:"rendered_prompt"`
}

type normalizedFeaturePromptSaveInput struct {
	Prompt                string
	Enabled               bool
	Variables             []FeaturePromptVariable
	ModelDefaults         FeaturePromptModelDefaults
	UserPromptMergePolicy FeaturePromptMergePolicy
}

type promptEvalFixture struct {
	ID                string
	Name              string
	BasePrompt        string
	UserPrompt        string
	Params            map[string]interface{}
	ExpectedContains  []string
	ForbiddenContains []string
}

func (r *FeaturePromptRepository) EvaluatePromptDraft(ctx context.Context, slug string, input FeaturePromptSaveInput, updatedBy *int64) (*FeaturePromptEvalReport, error) {
	normalized, err := normalizeFeaturePromptSaveInput(slug, input)
	if err != nil {
		return nil, err
	}
	return r.evaluateNormalizedPromptDraft(ctx, slug, normalized, updatedBy)
}

func (r *FeaturePromptRepository) GetLatestPromptEvalReport(ctx context.Context, slug string) (*FeaturePromptEvalReport, error) {
	row, err := r.q.GetSystemConfig(ctx, promptEvalLatestConfigKey(slug))
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return evalReportFromSystemConfig(row)
}

func (r *FeaturePromptRepository) listLatestPromptEvalReports(ctx context.Context) (map[string]*FeaturePromptEvalReport, error) {
	rows, err := r.q.ListSystemConfigsByPrefix(ctx, promptEvalLatestConfigKeyPrefix+"%")
	if err != nil {
		return nil, err
	}
	reports := make(map[string]*FeaturePromptEvalReport, len(rows))
	for _, row := range rows {
		report, err := evalReportFromSystemConfig(row)
		if err != nil {
			return nil, err
		}
		reports[report.FeatureSlug] = report
	}
	return reports, nil
}

func (r *FeaturePromptRepository) getPassingPromptEvalReportByDraftHash(ctx context.Context, slug string, draftHash string) (*FeaturePromptEvalReport, error) {
	report, err := r.getPromptEvalReportByDraftHash(ctx, slug, draftHash)
	if err != nil || report == nil {
		return report, err
	}
	if !report.Passed {
		return nil, nil
	}
	return report, nil
}

func (r *FeaturePromptRepository) getPromptEvalReportByDraftHash(ctx context.Context, slug string, draftHash string) (*FeaturePromptEvalReport, error) {
	row, err := r.q.GetSystemConfig(ctx, promptEvalDraftConfigKey(slug, draftHash))
	if err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	report, err := evalReportFromSystemConfig(row)
	if err != nil {
		return nil, err
	}
	return report, nil
}

func (r *FeaturePromptRepository) evaluateNormalizedPromptDraft(ctx context.Context, slug string, input normalizedFeaturePromptSaveInput, updatedBy *int64) (*FeaturePromptEvalReport, error) {
	report, err := buildFeaturePromptEvalReport(slug, input)
	if err != nil {
		return nil, err
	}
	report.CreatedBy = updatedBy
	report.CreatedAt = time.Now().UTC()
	report.ID = promptEvalReportID(report.DraftHash, report.CreatedAt)
	rawReport, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}

	err = dbutil.WithTx(ctx, r.db, func(q *dbsqlc.Queries) error {
		if err := q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{
			ConfigKey: promptEvalDraftConfigKey(slug, report.DraftHash),
			Value:     string(rawReport),
		}); err != nil {
			return err
		}
		return q.UpsertSystemConfig(ctx, dbsqlc.UpsertSystemConfigParams{
			ConfigKey: promptEvalLatestConfigKey(slug),
			Value:     string(rawReport),
		})
	})
	if err != nil {
		return nil, err
	}
	return report, nil
}

func buildFeaturePromptEvalReport(slug string, input normalizedFeaturePromptSaveInput) (*FeaturePromptEvalReport, error) {
	fixtures, suiteID := promptEvalFixturesForFeature(slug)
	if len(fixtures) == 0 {
		return nil, ErrPromptEvalUnsupported
	}

	draftHash, err := featurePromptDraftHash(slug, input)
	if err != nil {
		return nil, err
	}

	results := make([]FeaturePromptEvalFixtureResult, 0, len(fixtures))
	passedCount := 0
	for _, fixture := range fixtures {
		result := runPromptEvalFixture(slug, input, fixture)
		if result.Passed {
			passedCount++
		}
		results = append(results, result)
	}

	passed := passedCount == len(results)
	summary := fmt.Sprintf("%d/%d golden fixtures passed", passedCount, len(results))
	return &FeaturePromptEvalReport{
		FeatureSlug:    slug,
		DraftHash:      draftHash,
		SuiteID:        suiteID,
		Passed:         passed,
		Summary:        summary,
		FixtureResults: results,
	}, nil
}

func runPromptEvalFixture(slug string, input normalizedFeaturePromptSaveInput, fixture promptEvalFixture) FeaturePromptEvalFixtureResult {
	rendered := ApplyFeatureSystemPromptWithConfig(
		fixture.BasePrompt,
		fixture.UserPrompt,
		slug,
		fixture.Params,
		FeaturePromptRuntimeConfig{
			Prompt:                input.Prompt,
			ModelDefaults:         input.ModelDefaults,
			UserPromptMergePolicy: input.UserPromptMergePolicy,
		},
	)

	missingExpected := missingSubstrings(rendered, fixture.ExpectedContains)
	foundForbidden := foundSubstrings(rendered, fixture.ForbiddenContains)
	return FeaturePromptEvalFixtureResult{
		FixtureID:         fixture.ID,
		Name:              fixture.Name,
		Passed:            len(missingExpected) == 0 && len(foundForbidden) == 0,
		ExpectedContains:  append([]string(nil), fixture.ExpectedContains...),
		MissingExpected:   missingExpected,
		ForbiddenContains: append([]string(nil), fixture.ForbiddenContains...),
		FoundForbidden:    foundForbidden,
		RenderedPrompt:    rendered,
	}
}

func promptEvalFixturesForFeature(slug string) ([]promptEvalFixture, string) {
	if slug != workflow.FeatureSlugPatternGen {
		return nil, ""
	}
	return []promptEvalFixture{
		{
			ID:         "pattern-gen-user-intent",
			Name:       "Pattern generation preserves the Admin user's prompt intent",
			BasePrompt: "red floral silk scarf pattern, flat lay",
			UserPrompt: "red floral silk scarf pattern, flat lay",
			ExpectedContains: []string{
				"red floral silk scarf pattern, flat lay",
			},
			ForbiddenContains: []string{"{{"},
		},
	}, promptEvalSuitePatternGenV1
}

func promptEvalRequiredForDraft(slug string, input normalizedFeaturePromptSaveInput) bool {
	return slug == workflow.FeatureSlugPatternGen && input.Enabled
}

func promptEvalSupportedForFeature(slug string) bool {
	fixtures, _ := promptEvalFixturesForFeature(slug)
	return len(fixtures) > 0
}

func missingSubstrings(text string, expected []string) []string {
	missing := []string{}
	for _, value := range expected {
		if !strings.Contains(text, value) {
			missing = append(missing, value)
		}
	}
	return missing
}

func foundSubstrings(text string, forbidden []string) []string {
	found := []string{}
	for _, value := range forbidden {
		if strings.Contains(text, value) {
			found = append(found, value)
		}
	}
	return found
}

func normalizeFeaturePromptSaveInput(slug string, input FeaturePromptSaveInput) (normalizedFeaturePromptSaveInput, error) {
	mergePolicy := DefaultFeaturePromptMergePolicy(slug)
	if input.UserPromptMergePolicy != nil {
		mergePolicy = *input.UserPromptMergePolicy
	}
	return normalizedFeaturePromptSaveInput{
		Prompt:                input.Prompt,
		Enabled:               input.Enabled,
		Variables:             normalizePromptVariables(input.Variables),
		ModelDefaults:         input.ModelDefaults,
		UserPromptMergePolicy: mergePolicy,
	}, nil
}

func featurePromptDraftHash(slug string, input normalizedFeaturePromptSaveInput) (string, error) {
	payload := struct {
		FeatureSlug           string                     `json:"feature_slug"`
		Prompt                string                     `json:"prompt"`
		Enabled               bool                       `json:"enabled"`
		Variables             []FeaturePromptVariable    `json:"variables"`
		ModelDefaults         FeaturePromptModelDefaults `json:"model_defaults"`
		UserPromptMergePolicy FeaturePromptMergePolicy   `json:"user_prompt_merge_policy"`
	}{
		FeatureSlug:           slug,
		Prompt:                input.Prompt,
		Enabled:               input.Enabled,
		Variables:             input.Variables,
		ModelDefaults:         input.ModelDefaults,
		UserPromptMergePolicy: input.UserPromptMergePolicy,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func evalReportFromSystemConfig(row dbsqlc.SystemConfig) (*FeaturePromptEvalReport, error) {
	var report FeaturePromptEvalReport
	if err := json.Unmarshal([]byte(row.Value), &report); err != nil {
		return nil, err
	}
	return &report, nil
}

func promptEvalLatestConfigKey(slug string) string {
	return promptEvalLatestConfigKeyPrefix + slug
}

func promptEvalDraftConfigKey(slug string, draftHash string) string {
	return promptEvalDraftConfigKeyPrefix + slug + ":" + draftHash
}

func promptEvalReportID(draftHash string, createdAt time.Time) int64 {
	sum := sha256.Sum256([]byte(draftHash + ":" + strconv.FormatInt(createdAt.UnixNano(), 10)))
	var id uint64
	for _, b := range sum[:8] {
		id = (id << 8) | uint64(b)
	}
	const maxInt64 = uint64(1<<63 - 1)
	return int64(id%maxInt64) + 1
}

func promptEvalGateError(report *FeaturePromptEvalReport) error {
	return &PromptEvalGateError{Report: report}
}

func isPromptEvalGateError(err error) (*PromptEvalGateError, bool) {
	var gateErr *PromptEvalGateError
	if errors.As(err, &gateErr) {
		return gateErr, true
	}
	return nil, false
}
