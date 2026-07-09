package taskservice

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/boards"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/identity/users"
	"ideaboard/api/internal/media/storage"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/platform/validation"
	"ideaboard/api/internal/prompts"
	"ideaboard/api/internal/tasking/taskmedia"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/tasking/taskstore"
	"ideaboard/api/internal/testdb"
	"ideaboard/api/internal/testsupport/secretstest"
	"ideaboard/api/internal/workflow"
)

type testPublicURLProvider struct {
	setting settings.PublicURLSetting
}

func (p testPublicURLProvider) GetPublicURL(context.Context) (settings.PublicURLSetting, error) {
	return p.setting, nil
}

type testGoogleNativeGenerationSettings struct {
	setting settings.GoogleNativeGenerationSetting
	err     error
}

func (p *testGoogleNativeGenerationSettings) GetGoogleNativeGeneration(context.Context) (settings.GoogleNativeGenerationSetting, error) {
	if p.err != nil {
		return settings.GoogleNativeGenerationSetting{}, p.err
	}
	return p.setting, nil
}

type testSubmitMediaValidator struct{}

func (testSubmitMediaValidator) ValidateSubmitMediaRef(context.Context, int64, string) error {
	return nil
}

func TestApplyTaskSystemPromptUsesAdminConfiguredPromptOnly(t *testing.T) {
	tests := []struct {
		name       string
		feature    string
		basePrompt string
		userPrompt string
		params     map[string]interface{}
		template   string
		want       string
	}{
		{
			name:       "style fusion",
			feature:    workflow.FeatureSlugStyleFusion,
			basePrompt: "base fusion prompt",
			userPrompt: "融合但不要太夸张",
			params: map[string]interface{}{
				"fusion": map[string]interface{}{
					"input_weights": []interface{}{0.6, 0.4},
				},
			},
			template: "SYS {{user_prompt}} weights={{fusion_input_weights}}",
			want:     "SYS 融合但不要太夸张 weights=Image 1: 60%, Image 2: 40%",
		},
		{
			name:       "canvas extend",
			feature:    workflow.FeatureSlugImageExtend,
			basePrompt: "user prompt",
			userPrompt: "右侧延展为工作室背景",
			params: map[string]interface{}{
				"extend": map[string]interface{}{
					"top_percent":    0,
					"right_percent":  22,
					"bottom_percent": 0,
					"left_percent":   0,
				},
			},
			template: "SYS {{user_prompt}} top={{extend_top_percent}} right={{extend_right_percent}} bottom={{extend_bottom_percent}} left={{extend_left_percent}}",
			want:     "SYS 右侧延展为工作室背景 top=0 right=22 bottom=0 left=0",
		},
		{
			name:       "hd-upscale",
			feature:    workflow.FeatureSlugHDUpscale,
			basePrompt: "user prompt",
			userPrompt: "",
			params: map[string]interface{}{
				"hd": map[string]interface{}{
					"clarity_strength": 90,
				},
			},
			template: "SYS clarity={{clarity_strength}}",
			want:     "SYS clarity=90",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyTaskSystemPrompt(tt.basePrompt, tt.userPrompt, tt.feature, tt.params, func(string) (string, error) {
				return tt.template, nil
			})

			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if strings.Contains(got, "【执行语义】") {
				t.Fatalf("got unexpected hidden constraint in prompt: %q", got)
			}
		})
	}
}

func TestNormalizeTaskListKeywordTruncatesByRune(t *testing.T) {
	raw := strings.Repeat("画", maxTaskKeywordLength+8)

	got := normalizeTaskListKeyword(raw)

	if !utf8.ValidString(got) {
		t.Fatalf("keyword is not valid utf8")
	}
	if gotRunes := utf8.RuneCountInString(got); gotRunes != maxTaskKeywordLength {
		t.Fatalf("keyword rune count=%d, want %d", gotRunes, maxTaskKeywordLength)
	}
}

func TestNormalizeTaskListStatus(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: "", want: ""},
		{name: "trim and lower", raw: " Running ", want: tasks.StatusRunning},
		{name: "completed", raw: tasks.StatusCompleted, want: tasks.StatusCompleted},
		{name: "invalid", raw: "all", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeTaskListStatus(tt.raw)
			if tt.wantErr {
				if !validation.Is(err) {
					t.Fatalf("err=%v, want task list validation error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize status failed: %v", err)
			}
			if got != tt.want {
				t.Fatalf("status=%q, want %q", got, tt.want)
			}
		})
	}
}

func TestImageExtendEmptyUserPromptCanStillBuildExecutablePrompt(t *testing.T) {
	fullPrompt := applyTaskSystemPrompt(
		"",
		"",
		workflow.FeatureSlugImageExtend,
		map[string]interface{}{
			"extend": map[string]interface{}{
				"top_percent":    0,
				"right_percent":  30,
				"bottom_percent": 0,
				"left_percent":   0,
			},
		},
		func(string) (string, error) {
			return "SYS 扩图 right={{extend_right_percent}}", nil
		},
	)

	if strings.TrimSpace(fullPrompt) == "" {
		t.Fatalf("expected non-empty full prompt")
	}
	if err := validateExecutablePrompt(fullPrompt); err != nil {
		t.Fatalf("expected executable prompt, got err: %v", err)
	}
}

func TestRemoveBgWithoutConfiguredSystemPromptIsRejected(t *testing.T) {
	fullPrompt := applyTaskSystemPrompt(
		"",
		"",
		workflow.FeatureSlugRemoveBg,
		map[string]interface{}{
			"feature": workflow.FeatureSlugRemoveBg,
		},
		func(string) (string, error) {
			return "", nil
		},
	)

	if err := validateExecutablePrompt(fullPrompt); err == nil {
		t.Fatalf("expected validation error when remove-bg has no executable prompt")
	}
}

func TestValidateExecutablePromptReturnsExplicitErrorWhenFinalPromptEmpty(t *testing.T) {
	err := validateExecutablePrompt("   ")
	if err == nil {
		t.Fatalf("expected validation error for empty prompt")
	}
	want := "缺少提示词，请填写 Prompt 或配置后台 system prompt"
	if err.Error() != want {
		t.Fatalf("got %q, want %q", err.Error(), want)
	}
}

func TestSubmitTaskInputLimits(t *testing.T) {
	t.Run("prompt byte limit", func(t *testing.T) {
		err := validateSubmitTaskPrompt(strings.Repeat("a", maxSubmitTaskPromptBytes+1), maxSubmitTaskPromptBytes, "提示词")
		if err == nil {
			t.Fatalf("expected prompt limit error")
		}
	})

	t.Run("image url count limit", func(t *testing.T) {
		urls := make([]string, maxSubmitTaskImageURLs+1)
		for i := range urls {
			urls[i] = "https://example.com/image.png"
		}
		if err := validateSubmitTaskImageURLs(urls); err == nil {
			t.Fatalf("expected image url count error")
		}
	})

	t.Run("params image_urls count limit", func(t *testing.T) {
		urls := make([]interface{}, maxSubmitTaskImageURLs+1)
		for i := range urls {
			urls[i] = "https://example.com/image.png"
		}
		err := validateSubmitTaskMediaParams(map[string]interface{}{
			"image_urls": urls,
		})
		if err == nil {
			t.Fatalf("expected params image_urls count error")
		}
	})

	t.Run("params mask_image_urls count limit", func(t *testing.T) {
		urls := make([]interface{}, maxSubmitTaskImageURLs+1)
		for i := range urls {
			urls[i] = "https://example.com/image.png"
		}
		err := validateSubmitTaskMediaParams(map[string]interface{}{
			"mask_image_urls": urls,
		})
		if err == nil {
			t.Fatalf("expected params mask_image_urls count error")
		}
	})

	t.Run("params reference_image_urls count limit", func(t *testing.T) {
		urls := make([]interface{}, maxSubmitTaskImageURLs+1)
		for i := range urls {
			urls[i] = "https://example.com/image.png"
		}
		err := validateSubmitTaskMediaParams(map[string]interface{}{
			"reference_image_urls": urls,
		})
		if err == nil {
			t.Fatalf("expected params reference_image_urls count error")
		}
	})

	t.Run("reject non-structured feature params", func(t *testing.T) {
		tests := []string{
			"variation_strength",
			"extend_top",
			"extend_right",
			"extend_bottom",
			"extend_left",
			"clarity_strength",
			"fusion_input_weights",
		}
		for _, key := range tests {
			t.Run(key, func(t *testing.T) {
				err := validateSubmitTaskMediaParams(map[string]interface{}{
					key: 42,
				})
				if err == nil {
					t.Fatalf("expected structured param validation error")
				}
			})
		}
	})

	t.Run("params byte limit", func(t *testing.T) {
		_, err := marshalSubmitTaskParams(map[string]interface{}{
			"payload": strings.Repeat("x", maxSubmitTaskParamsBytes+1),
		})
		if err == nil {
			t.Fatalf("expected params size error")
		}
	})

	t.Run("params depth limit", func(t *testing.T) {
		var current interface{} = "leaf"
		for i := 0; i < maxSubmitTaskParamsDepth+1; i++ {
			current = map[string]interface{}{"next": current}
		}
		_, err := marshalSubmitTaskParams(map[string]interface{}{"root": current})
		if err == nil {
			t.Fatalf("expected params depth error")
		}
	})

	t.Run("reject private image url", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("http://127.0.0.1:8080/a.png"); err == nil {
			t.Fatal("expected private url to be rejected")
		}
	})

	t.Run("allow structured uploads relative path", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("/uploads/canvas/OWNR1A/board-1/content/a.png"); err != nil {
			t.Fatalf("expected structured upload path to pass: %v", err)
		}
	})

	t.Run("reject generic upload path", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("/uploads/2026/05/13/a.png"); err == nil {
			t.Fatal("expected generic upload path to fail")
		}
	})

	t.Run("reject non upload relative path", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("/tmp/a.png"); err == nil {
			t.Fatal("expected non upload relative path to fail")
		}
	})

	t.Run("reject oversized data url", func(t *testing.T) {
		tooLarge := "data:image/png;base64," + strings.Repeat("a", taskmedia.MaxSubmitTaskMediaDataURLBytes)
		if err := validateSubmitTaskMediaRef(tooLarge); err == nil {
			t.Fatal("expected oversized data url to fail")
		}
	})

	t.Run("normalize image and reference urls keeps semantic split", func(t *testing.T) {
		params := map[string]interface{}{
			"image_urls": []interface{}{
				"https://example.com/original.png",
				" ",
			},
			"reference_image_urls": []interface{}{
				"https://example.com/ref-a.png",
				"",
				"https://example.com/ref-b.png",
			},
		}
		normalizeSubmitTaskMediaParams(params)

		imageURLs, ok := params["image_urls"].([]string)
		if !ok {
			t.Fatalf("expected normalized image_urls []string, got %#v", params["image_urls"])
		}
		wantImageURLs := []string{"https://example.com/original.png"}
		if strings.Join(imageURLs, ",") != strings.Join(wantImageURLs, ",") {
			t.Fatalf("unexpected image_urls: got=%#v want=%#v", imageURLs, wantImageURLs)
		}

		referenceURLs, ok := params["reference_image_urls"].([]string)
		if !ok {
			t.Fatalf("expected normalized reference_image_urls []string, got %#v", params["reference_image_urls"])
		}
		wantReferenceURLs := []string{
			"https://example.com/ref-a.png",
			"https://example.com/ref-b.png",
		}
		if strings.Join(referenceURLs, ",") != strings.Join(wantReferenceURLs, ",") {
			t.Fatalf("unexpected reference_image_urls: got=%#v want=%#v", referenceURLs, wantReferenceURLs)
		}
	})
}

func TestNormalizeStyleFusionParams(t *testing.T) {
	t.Run("normalizes canonical weights", func(t *testing.T) {
		params := map[string]interface{}{
			"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
			"fusion": map[string]interface{}{
				"input_weights": []interface{}{2.0, 1.0},
			},
		}

		if err := normalizeStyleFusionParams(params); err != nil {
			t.Fatalf("normalizeStyleFusionParams returned error: %v", err)
		}
		fusion, ok := params["fusion"].(map[string]interface{})
		if !ok {
			t.Fatalf("fusion = %#v, want map", params["fusion"])
		}
		if fusion["mode"] != "weighted_reference" {
			t.Fatalf("mode = %#v, want weighted_reference", fusion["mode"])
		}
		weights, ok := fusion["input_weights"].([]float64)
		if !ok {
			t.Fatalf("input_weights = %#v, want []float64", fusion["input_weights"])
		}
		if len(weights) != 2 ||
			math.Abs(weights[0]-2.0/3.0) > 0.000001 ||
			math.Abs(weights[1]-1.0/3.0) > 0.000001 {
			t.Fatalf("weights = %#v, want normalized 2:1", weights)
		}
	})

	tests := []struct {
		name   string
		params map[string]interface{}
	}{
		{
			name: "rejects image count below range",
			params: map[string]interface{}{
				"image_urls": []string{"https://example.com/a.png"},
				"fusion": map[string]interface{}{
					"input_weights": []interface{}{1.0},
				},
			},
		},
		{
			name: "rejects missing fusion object",
			params: map[string]interface{}{
				"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
			},
		},
		{
			name: "rejects weight count mismatch",
			params: map[string]interface{}{
				"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
				"fusion": map[string]interface{}{
					"input_weights": []interface{}{1.0},
				},
			},
		},
		{
			name: "rejects negative weight",
			params: map[string]interface{}{
				"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
				"fusion": map[string]interface{}{
					"input_weights": []interface{}{1.0, -1.0},
				},
			},
		},
		{
			name: "rejects zero total weight",
			params: map[string]interface{}{
				"image_urls": []string{"https://example.com/a.png", "https://example.com/b.png"},
				"fusion": map[string]interface{}{
					"input_weights": []interface{}{0.0, 0.0},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := normalizeStyleFusionParams(tt.params); err == nil {
				t.Fatalf("expected normalizeStyleFusionParams error")
			}
		})
	}
}

func TestWorkflowTaskParamsFromMapPreservesExtraAndFusionContract(t *testing.T) {
	params := map[string]interface{}{
		"feature": "style-fusion",
		"model":   "gpt-image-1",
		"image_urls": []string{
			"https://example.com/a.png",
			"https://example.com/b.png",
		},
		"fusion": map[string]interface{}{
			"mode":          "weighted_reference",
			"input_weights": []interface{}{0.7, 0.3},
		},
		"provider_only": "keep-me",
	}

	typedParams, err := workflowTaskParamsFromMap(params, workflow.FeatureSlugStyleFusion)
	if err != nil {
		t.Fatalf("workflowTaskParamsFromMap returned error: %v", err)
	}
	if typedParams.SchemaVersion != tasks.CurrentParamsSchemaVersion {
		t.Fatalf("schema version = %d, want %d", typedParams.SchemaVersion, tasks.CurrentParamsSchemaVersion)
	}
	if typedParams.Fusion == nil {
		t.Fatal("Fusion params are nil")
	}
	if typedParams.Fusion.Mode != "weighted_reference" {
		t.Fatalf("fusion mode = %q, want weighted_reference", typedParams.Fusion.Mode)
	}
	if len(typedParams.Fusion.InputWeights) != 2 || typedParams.Fusion.InputWeights[0] != 0.7 || typedParams.Fusion.InputWeights[1] != 0.3 {
		t.Fatalf("fusion input weights = %#v, want [0.7 0.3]", typedParams.Fusion.InputWeights)
	}
	if typedParams.Media == nil || len(typedParams.Media.Inputs) != 2 {
		t.Fatalf("media inputs = %#v, want two inputs", typedParams.Media)
	}
	if typedParams.Media.Inputs[0].Weight != 0.7 || typedParams.Media.Inputs[1].Weight != 0.3 {
		t.Fatalf("media input weights = %#v, want fusion weights", typedParams.Media.Inputs)
	}
	if got := typedParams.Extra["provider_only"]; got != "keep-me" {
		t.Fatalf("provider_only extra = %#v, want keep-me", got)
	}
}

func TestWorkflowTaskParamsFromMapDoesNotPreserveKnownContractKeysInExtra(t *testing.T) {
	params := map[string]interface{}{
		"feature": "image-extend",
		"model":   "gpt-image-1",
		"image_urls": []string{
			"https://example.com/source.png",
		},
		"reference_image_urls": []string{
			"https://example.com/ref.png",
		},
		"mask_image_url":          "https://example.com/mask.png",
		"original_image_url":      "https://example.com/source.png",
		"use_mask":                true,
		"custom_width":            640,
		"custom_height":           480,
		"original_image_width":    1024,
		"original_image_height":   768,
		"ratio":                   "4:3",
		"quality":                 "high",
		"sample_count":            1,
		"output_count":            2,
		"image_count":             2,
		"prompt_upsampling":       false,
		"extend":                  map[string]interface{}{"top_percent": 0, "right_percent": 25, "bottom_percent": 0, "left_percent": 0},
		"provider_specific_param": "keep-me",
	}

	typedParams, err := workflowTaskParamsFromMap(params, workflow.FeatureSlugImageExtend)
	if err != nil {
		t.Fatalf("workflowTaskParamsFromMap returned error: %v", err)
	}
	if typedParams.Extra == nil {
		t.Fatalf("Extra is nil, want provider-specific param preserved")
	}
	if got := typedParams.Extra["provider_specific_param"]; got != "keep-me" {
		t.Fatalf("provider_specific_param extra = %#v, want keep-me", got)
	}
	for _, key := range []string{
		"feature",
		"model",
		"image_urls",
		"reference_image_urls",
		"mask_image_url",
		"original_image_url",
		"use_mask",
		"custom_width",
		"custom_height",
		"original_image_width",
		"original_image_height",
		"ratio",
		"quality",
		"sample_count",
		"output_count",
		"image_count",
		"prompt_upsampling",
		"extend",
	} {
		if _, exists := typedParams.Extra[key]; exists {
			t.Fatalf("known submit param %s leaked into Extra: %#v", key, typedParams.Extra)
		}
	}
	if typedParams.Media == nil || typedParams.Media.Mask == nil || typedParams.Media.Original == nil || typedParams.Media.UseMask == nil {
		t.Fatalf("media = %#v, want input/reference/mask/original/use_mask", typedParams.Media)
	}
	if typedParams.Controls == nil || typedParams.Controls.CustomWidth != 640 || typedParams.Controls.OutputCount != 2 {
		t.Fatalf("controls = %#v, want canonical controls", typedParams.Controls)
	}
	if typedParams.Extend == nil || typedParams.Extend.RightPercent != 25 {
		t.Fatalf("extend = %#v, want right_percent 25", typedParams.Extend)
	}
}

func TestSubmitTaskCanvasUploadRefsRequireBoardAccess(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	userRepo := users.NewUserRepository(db)
	owner := mustCreateTaskUploadUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateTaskUploadUser(t, userRepo, "member", "member-auth", "MEMB1A")
	outsider := mustCreateTaskUploadUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")
	boardRepo := boards.NewBoardRepository(db)
	now := time.Now()
	if err := boardRepo.Upsert(ctx, &boards.UserBoard{BoardID: "board-1", OwnerUserID: owner.ID, CreatedBy: owner.ID, Name: "Demo", State: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if err := boardRepo.UpsertMember(ctx, owner.ID, "board-1", member.ID, boards.BoardAccessRoleViewer); err != nil {
		t.Fatalf("create board member failed: %v", err)
	}

	mediaAuthorizer := storage.NewTaskMediaAuthorizer(
		testTaskUploadBoardAccessResolver{service: boards.NewBoardService(boardRepo, userRepo)},
		testTaskUploadOwnerStorageUIDResolver{repo: userRepo},
	)
	svc := NewService(ServiceDeps{
		MediaValidator: taskmedia.NewSubmitMediaValidator(
			testPublicURLProvider{setting: settings.PublicURLSetting{PublicAssetBaseURL: "https://assets.example.com"}},
			mediaAuthorizer,
		),
	}, ServiceOptions{})
	allowed := "/uploads/canvas/OWNR1A/board-1/content/demo.png"
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, allowed); err != nil {
		t.Fatalf("expected member to use canvas upload: %v", err)
	}
	absoluteAllowed := "https://assets.example.com/uploads/canvas/OWNR1A/board-1/content/demo.png"
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, absoluteAllowed); err != nil {
		t.Fatalf("expected member to use public canvas upload URL: %v", err)
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, outsider.ID, absoluteAllowed); err == nil {
		t.Fatalf("expected outsider to be rejected for public canvas upload URL")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, outsider.ID, allowed); err == nil {
		t.Fatalf("expected outsider to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/canvas/WRONG1/board-1/content/demo.png"); err == nil {
		t.Fatalf("expected owner segment mismatch to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/2026/05/13/demo.png"); err == nil {
		t.Fatalf("expected generic upload path to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/profile/MEMB1A/avatar/demo.png"); err == nil {
		t.Fatalf("expected non-canvas upload path to be rejected")
	}
}

func TestSubmitTaskMediaRefValidationFailsClosedWithoutServiceValidator(t *testing.T) {
	ctx := context.Background()
	const allowedByFormat = "/uploads/canvas/OWNR1A/board-1/content/demo.png"
	want := "任务媒体校验服务未配置"

	tests := []struct {
		name string
		svc  *Service
	}{
		{name: "nil service"},
		{name: "nil media validator", svc: NewService(ServiceDeps{}, ServiceOptions{})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.svc.validateSubmitTaskMediaRef(ctx, 42, allowedByFormat)
			if err == nil {
				t.Fatalf("expected missing validator to fail closed")
			}
			if err.Error() != want {
				t.Fatalf("error = %q, want %q", err.Error(), want)
			}
		})
	}
}

func TestSubmitTaskUsesSubmissionStore(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{ID: 101, Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}}},
		{ID: 202, Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}}},
	}}
	submissionStore := &recordingSubmissionStore{}
	taskEvents := &recordingTaskStatusPublisher{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
	}, ServiceOptions{
		TaskEvents: taskEvents,
	})

	taskID, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:         "draw a chair",
		ClientActionID: " canvas-action-1 ",
		Params:         map[string]interface{}{"size": "1024x1024"},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if taskID == "" {
		t.Fatalf("expected task id")
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.task.TaskID != taskID || call.task.UserID != 42 || call.task.Status != tasks.StatusPending {
		t.Fatalf("unexpected task: %+v", call.task)
	}
	if call.task.ClientActionID != "canvas-action-1" {
		t.Fatalf("client_action_id=%q, want normalized canvas-action-1", call.task.ClientActionID)
	}
	if call.task.Priority != tasks.TaskPriorityDefault || call.queueTask.Priority != tasks.TaskPriorityDefault {
		t.Fatalf("priority task=%d queue=%d, want default %d", call.task.Priority, call.queueTask.Priority, tasks.TaskPriorityDefault)
	}
	if call.queueTask.TaskID != taskID || call.queueTask.UserID != 42 {
		t.Fatalf("unexpected queue task: %+v", call.queueTask)
	}
	if !slices.Equal(call.queueTask.AccountIDs, []int64{101, 202}) {
		t.Fatalf("queue account ids=%v, want [101 202]", call.queueTask.AccountIDs)
	}
	if len(taskEvents.events) != 1 || taskEvents.events[0].TaskID != taskID || taskEvents.events[0].Status != tasks.StatusPending {
		t.Fatalf("unexpected task events: %+v", taskEvents.events)
	}
}

func TestSubmitTaskGoogleNativeGenerationSettingControlsSubmission(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		setting     settings.GoogleNativeGenerationSetting
		wantAllowed bool
		wantMessage string
	}{
		{
			name: "disabled",
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeDisabled,
				CanaryPercent: 100,
			},
			wantMessage: "Google AI 原生生成通道当前已关闭",
		},
		{
			name: "enabled",
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeEnabled,
				CanaryPercent: 0,
			},
			wantAllowed: true,
		},
		{
			name: "canary included",
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeCanary,
				CanaryPercent: 100,
			},
			wantAllowed: true,
		},
		{
			name: "canary excluded",
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeCanary,
				CanaryPercent: 0,
			},
			wantMessage: "Google AI 原生生成通道当前处于灰度开放",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			submissionStore := &recordingSubmissionStore{}
			svc := newGoogleNativeSubmitTestService(tt.setting, submissionStore)

			_, readinessErr := svc.CheckSubmitTaskReadiness(ctx, 42, newGoogleNativeSubmitRequest())
			taskID, submitErr := svc.SubmitTask(ctx, 42, newGoogleNativeSubmitRequest())
			if tt.wantAllowed {
				if readinessErr != nil {
					t.Fatalf("readiness error = %v, want nil", readinessErr)
				}
				if submitErr != nil {
					t.Fatalf("submit error = %v, want nil", submitErr)
				}
				if taskID == "" {
					t.Fatalf("taskID is empty")
				}
				if len(submissionStore.calls) != 1 {
					t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
				}
				if !slices.Equal(submissionStore.calls[0].queueTask.AccountIDs, []int64{101}) {
					t.Fatalf("account ids=%v, want only google native account", submissionStore.calls[0].queueTask.AccountIDs)
				}
				return
			}

			if readinessErr == nil || !strings.Contains(readinessErr.Error(), tt.wantMessage) {
				t.Fatalf("readiness error = %v, want %q", readinessErr, tt.wantMessage)
			}
			if submitErr == nil || !strings.Contains(submitErr.Error(), tt.wantMessage) {
				t.Fatalf("submit error = %v, want %q", submitErr, tt.wantMessage)
			}
			if taskID != "" {
				t.Fatalf("taskID=%q, want empty", taskID)
			}
			if len(submissionStore.calls) != 0 {
				t.Fatalf("submission calls=%d, want 0", len(submissionStore.calls))
			}
		})
	}
}

func TestSubmitTaskGoogleNativeGenerationSettingRollbackStopsNewTasks(t *testing.T) {
	ctx := context.Background()
	submissionStore := &recordingSubmissionStore{}
	generationSettings := &testGoogleNativeGenerationSettings{
		setting: settings.GoogleNativeGenerationSetting{
			Mode:          settings.GoogleNativeGenerationModeEnabled,
			CanaryPercent: 0,
		},
	}
	svc := newGoogleNativeSubmitTestServiceWithSettings(generationSettings, submissionStore)

	if _, err := svc.SubmitTask(ctx, 42, newGoogleNativeSubmitRequest()); err != nil {
		t.Fatalf("initial submit failed: %v", err)
	}
	generationSettings.setting = settings.GoogleNativeGenerationSetting{
		Mode:          settings.GoogleNativeGenerationModeDisabled,
		CanaryPercent: 0,
	}
	if _, err := svc.SubmitTask(ctx, 42, newGoogleNativeSubmitRequest()); err == nil || !strings.Contains(err.Error(), "Google AI 原生生成通道当前已关闭") {
		t.Fatalf("post-rollback submit error = %v, want disabled gate", err)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want only pre-rollback task", len(submissionStore.calls))
	}
}

func TestSubmitTaskGoogleNativeGenerationSettingDoesNotFallbackToNonGoogleAccount(t *testing.T) {
	ctx := context.Background()
	submissionStore := &recordingSubmissionStore{}
	svc := newGoogleNativeSubmitTestServiceWithAccounts(
		&testGoogleNativeGenerationSettings{setting: settings.GoogleNativeGenerationSetting{
			Mode:          settings.GoogleNativeGenerationModeDisabled,
			CanaryPercent: 0,
		}},
		submissionStore,
		[]*accounts.ApiAccount{
			{
				ID:      101,
				ApiType: workflow.ProviderGoogleAI,
				Models:  []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
			},
			{
				ID:      202,
				ApiType: workflow.ProviderOpenAICompatible,
				Models:  []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
			},
		},
	)

	if _, err := svc.CheckSubmitTaskReadiness(ctx, 42, newGoogleNativeSubmitRequest()); err == nil || !strings.Contains(err.Error(), "Google AI 原生生成通道当前已关闭") {
		t.Fatalf("readiness error = %v, want disabled gate", err)
	}
	if _, err := svc.SubmitTask(ctx, 42, newGoogleNativeSubmitRequest()); err == nil || !strings.Contains(err.Error(), "Google AI 原生生成通道当前已关闭") {
		t.Fatalf("submit error = %v, want disabled gate", err)
	}
	if len(submissionStore.calls) != 0 {
		t.Fatalf("submission calls=%d, want 0", len(submissionStore.calls))
	}
}

func TestSubmitTaskRejectsInvalidPatternGenContractBeforeSubmission(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name:         "gpt-image-1",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:  "repeatable textile pattern",
		Feature: workflow.FeatureSlugPatternGen,
		Params: map[string]interface{}{
			"ratio": "2:1",
		},
	})
	if err == nil {
		t.Fatalf("expected pattern-gen contract validation error")
	}
	if !strings.Contains(err.Error(), "图案生成尺寸比例仅支持 auto、1:1、3:4、4:3、9:16、16:9") {
		t.Fatalf("error=%q, want actionable pattern-gen ratio message", err.Error())
	}
	if len(submissionStore.calls) != 0 {
		t.Fatalf("submission calls=%d, want 0", len(submissionStore.calls))
	}
}

func TestSubmitTaskUsesInteractivePriority(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{ID: 101, Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}}},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:   "draw a chair",
		Priority: tasks.TaskPriorityInteractive,
		Params:   map[string]interface{}{"size": "1024x1024"},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.task.Priority != tasks.TaskPriorityInteractive || call.queueTask.Priority != tasks.TaskPriorityInteractive {
		t.Fatalf("priority task=%d queue=%d, want interactive %d", call.task.Priority, call.queueTask.Priority, tasks.TaskPriorityInteractive)
	}
}

func TestSubmitTaskRejectsInvalidPriority(t *testing.T) {
	ctx := context.Background()
	svc := NewService(ServiceDeps{}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:   "draw a chair",
		Priority: tasks.TaskPriorityLowest + 1,
		Params:   map[string]interface{}{"size": "1024x1024"},
	})
	if err == nil {
		t.Fatalf("expected priority validation error")
	}
}

func TestSubmitTaskReturnsExistingTaskWithoutPublishingWhenClientActionRepeats(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{ID: 101, Models: []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}}},
	}}
	submissionStore := &recordingSubmissionStore{
		result: tasks.TaskSubmissionResult{TaskID: "task_existing_action", Created: false},
	}
	taskEvents := &recordingTaskStatusPublisher{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
	}, ServiceOptions{
		TaskEvents: taskEvents,
	})

	taskID, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:         "draw a chair",
		ClientActionID: "canvas-action-repeat",
		Params:         map[string]interface{}{"size": "1024x1024"},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if taskID != "task_existing_action" {
		t.Fatalf("task_id=%q, want existing task id", taskID)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	if len(taskEvents.events) != 0 {
		t.Fatalf("task events=%+v, want none for existing task", taskEvents.events)
	}
}

func TestSubmitTaskRejectsInvalidClientActionID(t *testing.T) {
	ctx := context.Background()
	svc := NewService(ServiceDeps{}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:         "draw a chair",
		ClientActionID: strings.Repeat("x", maxSubmitTaskClientActionIDBytes+1),
		Params:         map[string]interface{}{"size": "1024x1024"},
	})
	if err == nil {
		t.Fatalf("expected client_action_id validation error")
	}
}

func TestSubmitTaskAppliesFeaturePromptModelDefaultBeforeRouting(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name:         "gpt-image-1",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		},
		{
			ID: 202,
			Models: []accounts.AccountModel{{
				Name:         "image-alpha",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
		FeaturePrompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugPatternGen: {
				Prompt:                "{{user_prompt}}",
				ModelDefaults:         prompts.FeaturePromptModelDefaults{Model: "image-alpha"},
				UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(workflow.FeatureSlugPatternGen),
			},
		}},
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:  "draw a chair",
		Feature: workflow.FeatureSlugPatternGen,
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.task.RequestedModel != "image-alpha" {
		t.Fatalf("requested model=%q, want image-alpha", call.task.RequestedModel)
	}
	if !slices.Equal(call.queueTask.AccountIDs, []int64{202}) {
		t.Fatalf("queue account ids=%v, want [202]", call.queueTask.AccountIDs)
	}
	typedParams, err := tasks.UnmarshalParams(call.task.Params)
	if err != nil {
		t.Fatalf("unmarshal task params: %v", err)
	}
	if typedParams.Model != "image-alpha" {
		t.Fatalf("typed model=%q, want image-alpha", typedParams.Model)
	}
}

func TestSubmitTaskExplicitModelOverridesFeaturePromptModelDefault(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name:         "gpt-image-1",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		},
		{
			ID: 202,
			Models: []accounts.AccountModel{{
				Name:         "image-alpha",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugPatternGen},
			}},
		},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
		FeaturePrompts: fakeFeaturePromptStore{configs: map[string]prompts.FeaturePromptRuntimeConfig{
			workflow.FeatureSlugPatternGen: {
				Prompt:                "{{user_prompt}}",
				ModelDefaults:         prompts.FeaturePromptModelDefaults{Model: "image-alpha"},
				UserPromptMergePolicy: prompts.DefaultFeaturePromptMergePolicy(workflow.FeatureSlugPatternGen),
			},
		}},
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:  "draw a chair",
		Feature: workflow.FeatureSlugPatternGen,
		Params:  map[string]interface{}{"model": "gpt-image-1"},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.task.RequestedModel != "gpt-image-1" {
		t.Fatalf("requested model=%q, want gpt-image-1", call.task.RequestedModel)
	}
	if !slices.Equal(call.queueTask.AccountIDs, []int64{101}) {
		t.Fatalf("queue account ids=%v, want [101]", call.queueTask.AccountIDs)
	}
}

func TestSubmitTaskNormalizesStyleFusionContract(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name:         "gpt-image-1",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugStyleFusion},
			}},
		},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:  "融合但不要太夸张",
		Feature: workflow.FeatureSlugStyleFusion,
		ImageURLs: []string{
			"https://example.com/a.png",
			"https://example.com/b.png",
		},
		Params: map[string]interface{}{
			"fusion": map[string]interface{}{
				"input_weights": []interface{}{2.0, 1.0},
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}
	if len(submissionStore.calls) != 1 {
		t.Fatalf("submission calls=%d, want 1", len(submissionStore.calls))
	}
	call := submissionStore.calls[0]
	if call.queueTask.Params != call.task.Params {
		t.Fatalf("queue params differ from task params: queue=%s task=%s", call.queueTask.Params, call.task.Params)
	}
	if call.queueTask.Prompt != call.task.Prompt {
		t.Fatalf("queue prompt=%q, want task prompt=%q", call.queueTask.Prompt, call.task.Prompt)
	}
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(call.task.Params), &params); err != nil {
		t.Fatalf("unmarshal task params: %v", err)
	}
	typedParams, err := tasks.UnmarshalParams(call.queueTask.Params)
	if err != nil {
		t.Fatalf("unmarshal queue params: %v", err)
	}
	if typedParams.Feature != workflow.FeatureSlugStyleFusion {
		t.Fatalf("typed feature=%q, want style-fusion", typedParams.Feature)
	}
	if typedParams.Media == nil || len(typedParams.Media.Inputs) != 2 {
		t.Fatalf("typed media inputs=%#v, want two inputs", typedParams.Media)
	}
	if math.Abs(typedParams.Media.Inputs[0].Weight-2.0/3.0) > 0.000001 ||
		math.Abs(typedParams.Media.Inputs[1].Weight-1.0/3.0) > 0.000001 {
		t.Fatalf("typed media weights=%#v, want normalized 2:1", typedParams.Media.Inputs)
	}
	if typedParams.Fusion == nil || typedParams.Fusion.Mode != "weighted_reference" {
		t.Fatalf("typed fusion=%#v, want weighted_reference", typedParams.Fusion)
	}
	fusion, ok := params["fusion"].(map[string]interface{})
	if !ok {
		t.Fatalf("fusion = %#v, want object", params["fusion"])
	}
	if fusion["mode"] != "weighted_reference" {
		t.Fatalf("fusion mode=%#v, want weighted_reference", fusion["mode"])
	}
	weights, ok := fusion["input_weights"].([]interface{})
	if !ok || len(weights) != 2 {
		t.Fatalf("fusion input_weights=%#v, want two weights", fusion["input_weights"])
	}
	if math.Abs(weights[0].(float64)-2.0/3.0) > 0.000001 ||
		math.Abs(weights[1].(float64)-1.0/3.0) > 0.000001 {
		t.Fatalf("fusion input_weights=%#v, want normalized 2:1", weights)
	}
	if params["feature"] != workflow.FeatureSlugStyleFusion {
		t.Fatalf("feature=%#v, want style-fusion", params["feature"])
	}
	if _, ok := params["fusion"].(map[string]interface{}); !ok {
		t.Fatalf("fusion = %#v, want nested object preserved", params["fusion"])
	}
	if _, exists := params["image_urls"]; exists {
		t.Fatalf("did not expect provider image_urls in stored params: %#v", params)
	}
	media, ok := params["media"].(map[string]interface{})
	if !ok {
		t.Fatalf("media = %#v, want object", params["media"])
	}
	inputs, ok := media["inputs"].([]interface{})
	if !ok || len(inputs) != 2 {
		t.Fatalf("media inputs = %#v, want two inputs", media["inputs"])
	}
}

func TestSubmitTaskNormalizesStructuredPromptContractParams(t *testing.T) {
	ctx := context.Background()
	accountRepo := &fakeSubmitAccountStore{accounts: []*accounts.ApiAccount{
		{
			ID: 101,
			Models: []accounts.AccountModel{{
				Name:         "gpt-image-1",
				Enabled:      true,
				FeatureSlugs: []string{workflow.FeatureSlugImageExtend},
			}},
		},
	}}
	submissionStore := &recordingSubmissionStore{}
	svc := NewService(ServiceDeps{
		AccountRepo:     accountRepo,
		SubmissionStore: submissionStore,
		MediaValidator:  testSubmitMediaValidator{},
	}, ServiceOptions{})

	_, err := svc.SubmitTask(ctx, 42, &tasks.SubmitTaskRequest{
		Prompt:  "右侧延展为工作室背景",
		Feature: workflow.FeatureSlugImageExtend,
		ImageURLs: []string{
			"https://example.com/source.png",
		},
		Params: map[string]interface{}{
			"extend": map[string]interface{}{
				"top_percent":    0,
				"right_percent":  25,
				"bottom_percent": 0,
				"left_percent":   0,
			},
		},
	})
	if err != nil {
		t.Fatalf("SubmitTask failed: %v", err)
	}

	var params map[string]interface{}
	if err := json.Unmarshal([]byte(submissionStore.calls[0].task.Params), &params); err != nil {
		t.Fatalf("unmarshal task params: %v", err)
	}
	extend, ok := params["extend"].(map[string]interface{})
	if !ok {
		t.Fatalf("extend = %#v, want object", params["extend"])
	}
	if extend["right_percent"] != float64(25) {
		t.Fatalf("right_percent = %#v, want 25", extend["right_percent"])
	}
	if _, exists := params["extend_right"]; exists {
		t.Fatalf("did not expect provider extend_right in stored params: %#v", params)
	}
	if _, exists := params["image_urls"]; exists {
		t.Fatalf("did not expect provider image_urls in stored params: %#v", params)
	}
	media, ok := params["media"].(map[string]interface{})
	if !ok {
		t.Fatalf("media = %#v, want object", params["media"])
	}
	inputs, ok := media["inputs"].([]interface{})
	if !ok || len(inputs) != 1 {
		t.Fatalf("media inputs = %#v, want one input", media["inputs"])
	}
}

type testTaskUploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r testTaskUploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.OwnerUserID, nil
}

type testTaskUploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r testTaskUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}

func mustCreateTaskUploadUser(t *testing.T, repo *users.UserRepository, username string, authUserID string, storageUID string) *users.User {
	t.Helper()
	user := &users.User{
		Username:   username,
		AuthUserID: &authUserID,
		StorageUID: storageUID,
		Role:       "user",
		Status:     1,
	}
	if err := repo.Create(context.Background(), user); err != nil {
		t.Fatalf("create user %s failed: %v", username, err)
	}
	return user
}

type fakeSubmitAccountStore struct {
	accounts []*accounts.ApiAccount
	err      error
}

func (s *fakeSubmitAccountStore) GetAllEnabled(context.Context) ([]*accounts.ApiAccount, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.accounts, nil
}

func newGoogleNativeSubmitTestService(setting settings.GoogleNativeGenerationSetting, submissionStore *recordingSubmissionStore) *Service {
	return newGoogleNativeSubmitTestServiceWithSettings(&testGoogleNativeGenerationSettings{setting: setting}, submissionStore)
}

func newGoogleNativeSubmitTestServiceWithSettings(generationSettings *testGoogleNativeGenerationSettings, submissionStore *recordingSubmissionStore) *Service {
	return newGoogleNativeSubmitTestServiceWithAccounts(generationSettings, submissionStore, []*accounts.ApiAccount{
		{
			ID:      101,
			ApiType: workflow.ProviderGoogleAI,
			Models:  []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
		},
	})
}

func newGoogleNativeSubmitTestServiceWithAccounts(generationSettings *testGoogleNativeGenerationSettings, submissionStore *recordingSubmissionStore, apiAccounts []*accounts.ApiAccount) *Service {
	return NewService(ServiceDeps{
		AccountRepo:        &fakeSubmitAccountStore{accounts: apiAccounts},
		SubmissionStore:    submissionStore,
		GenerationSettings: generationSettings,
	}, ServiceOptions{})
}

func newGoogleNativeSubmitRequest() *tasks.SubmitTaskRequest {
	return &tasks.SubmitTaskRequest{
		Prompt: "draw a product mockup",
		Params: map[string]interface{}{
			"model": "gemini-2.5-flash-image",
		},
	}
}

type fakeFeaturePromptStore struct {
	configs map[string]prompts.FeaturePromptRuntimeConfig
	err     error
}

func (s fakeFeaturePromptStore) GetPrompt(ctx context.Context, slug string) (string, error) {
	config, err := s.GetPromptRuntimeConfig(ctx, slug)
	if err != nil {
		return "", err
	}
	return config.Prompt, nil
}

func (s fakeFeaturePromptStore) GetPromptRuntimeConfig(_ context.Context, slug string) (prompts.FeaturePromptRuntimeConfig, error) {
	if s.err != nil {
		return prompts.FeaturePromptRuntimeConfig{}, s.err
	}
	if s.configs == nil {
		return prompts.FeaturePromptRuntimeConfig{}, errors.New("feature prompt not found")
	}
	config, ok := s.configs[slug]
	if !ok {
		return prompts.FeaturePromptRuntimeConfig{}, errors.New("feature prompt not found")
	}
	return config, nil
}

type recordingSubmissionStore struct {
	calls  []submitCall
	result tasks.TaskSubmissionResult
	err    error
}

type submitCall struct {
	task      *tasks.Task
	queueTask *tasks.TaskPayload
}

func (s *recordingSubmissionStore) SubmitTask(_ context.Context, task *tasks.Task, queueTask *tasks.TaskPayload) (tasks.TaskSubmissionResult, error) {
	if s.err != nil {
		return tasks.TaskSubmissionResult{}, s.err
	}
	s.calls = append(s.calls, submitCall{task: task, queueTask: queueTask})
	if s.result.TaskID != "" {
		return s.result, nil
	}
	return tasks.TaskSubmissionResult{TaskID: task.TaskID, Created: true}, nil
}

type recordingTaskStatusPublisher struct {
	events []events.TaskStatusEvent
}

func (p *recordingTaskStatusPublisher) PublishTaskStatus(_ context.Context, event events.TaskStatusEvent) error {
	p.events = append(p.events, events.NormalizeTaskStatusEvent(event))
	return nil
}

type fakeCancelStore struct {
	marked  []string
	markErr error
}

func (f *fakeCancelStore) MarkCancelled(_ context.Context, taskID string, _ time.Duration) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.marked = append(f.marked, taskID)
	return nil
}
func (f *fakeCancelStore) IsCancelled(context.Context, string) (bool, error) { return false, nil }
func (f *fakeCancelStore) ClearCancelled(context.Context, string) error      { return nil }

func TestCancelUserTaskMarksRunningTaskCancelledWithoutDeletingRow(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID:     "task_cancel_running",
		UserID:     42,
		UserPrompt: "test",
		Prompt:     "test",
		Status:     tasks.StatusRunning,
		Params:     "{}",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	cancelStore := &fakeCancelStore{}
	publisher := &recordingTaskStatusPublisher{}
	svc := NewService(ServiceDeps{
		TaskRepo:    taskRepo,
		CancelStore: cancelStore,
	}, ServiceOptions{TaskEvents: publisher})

	if err := svc.CancelUserTask(ctx, task.TaskID, task.UserID); err != nil {
		t.Fatalf("CancelUserTask failed: %v", err)
	}

	if len(cancelStore.marked) != 1 || cancelStore.marked[0] != task.TaskID {
		t.Fatalf("cancel markers=%+v, want %q", cancelStore.marked, task.TaskID)
	}
	stopped, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load stopped task: %v", err)
	}
	if stopped.Status != tasks.StatusCancelled {
		t.Fatalf("status=%q, want cancelled", stopped.Status)
	}
	if stopped.CancelledAt == nil || stopped.CompletedAt == nil {
		t.Fatalf("cancelled_at=%v completed_at=%v, want terminal timestamps", stopped.CancelledAt, stopped.CompletedAt)
	}
	if stopped.CancelledBy == nil || *stopped.CancelledBy != task.UserID {
		t.Fatalf("cancelled_by=%v, want %d", stopped.CancelledBy, task.UserID)
	}
	if stopped.CancelReason != userDeletedTaskCancelReason {
		t.Fatalf("cancel_reason=%q, want %q", stopped.CancelReason, userDeletedTaskCancelReason)
	}
	if len(publisher.events) != 1 || publisher.events[0].TaskID != task.TaskID || publisher.events[0].Status != tasks.StatusCancelled {
		t.Fatalf("published events=%+v, want one cancelled event", publisher.events)
	}
}

func TestCancelUserTaskDoesNotMarkTaskCancelledWhenCancellationMarkerFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID:     "task_cancel_marker_failed",
		UserID:     42,
		UserPrompt: "test",
		Prompt:     "test",
		Status:     tasks.StatusRunning,
		Params:     "{}",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	markerErr := errors.New("marker failed")
	cancelStore := &fakeCancelStore{markErr: markerErr}
	publisher := &recordingTaskStatusPublisher{}
	svc := NewService(ServiceDeps{
		TaskRepo:    taskRepo,
		CancelStore: cancelStore,
	}, ServiceOptions{TaskEvents: publisher})

	if err := svc.CancelUserTask(ctx, task.TaskID, task.UserID); !errors.Is(err, markerErr) {
		t.Fatalf("CancelUserTask error=%v, want marker error", err)
	}

	loaded, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if loaded.Status != tasks.StatusRunning {
		t.Fatalf("status=%q, want running", loaded.Status)
	}
	if loaded.CancelledAt != nil || loaded.CompletedAt != nil {
		t.Fatalf("cancelled_at=%v completed_at=%v, want unchanged task", loaded.CancelledAt, loaded.CompletedAt)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events=%+v, want none", publisher.events)
	}
}

func TestCancelUserTaskIsIdempotentForTerminalTask(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	completedAt := time.Now()
	task := &tasks.Task{
		TaskID:      "task_cancel_completed",
		UserID:      42,
		UserPrompt:  "test",
		Prompt:      "test",
		Status:      tasks.StatusCompleted,
		Params:      "{}",
		CompletedAt: &completedAt,
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	cancelStore := &fakeCancelStore{}
	publisher := &recordingTaskStatusPublisher{}
	svc := NewService(ServiceDeps{
		TaskRepo:    taskRepo,
		CancelStore: cancelStore,
	}, ServiceOptions{TaskEvents: publisher})

	if err := svc.CancelUserTask(ctx, task.TaskID, task.UserID); err != nil {
		t.Fatalf("CancelUserTask should be idempotent for terminal task: %v", err)
	}

	if len(cancelStore.marked) != 0 {
		t.Fatalf("cancel markers=%+v, want none for terminal task", cancelStore.marked)
	}
	loaded, err := taskRepo.GetByTaskID(ctx, task.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if loaded.Status != tasks.StatusCompleted {
		t.Fatalf("status=%q, want completed", loaded.Status)
	}
	if len(publisher.events) != 0 {
		t.Fatalf("published events=%+v, want none", publisher.events)
	}
}

type recordingTaskOutputCleaner struct {
	calls []taskOutputCleanerCall
	err   error
}

type taskOutputCleanerCall struct {
	userID int64
	taskID string
}

func (c *recordingTaskOutputCleaner) DeleteTaskOutputsByUserTask(_ context.Context, userID int64, taskID string) error {
	c.calls = append(c.calls, taskOutputCleanerCall{userID: userID, taskID: taskID})
	if c.err != nil {
		return c.err
	}
	return nil
}

func TestDeleteUserTaskCleansOutputsAndTaskOutputObjects(t *testing.T) {
	ctx := context.Background()
	secretstest.EnableEncryption(t)
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	accountRepo := accounts.NewAccountRepository(db)
	userRepo := users.NewUserRepository(db)
	owner := mustCreateTaskUploadUser(t, userRepo, "delete-output-owner", "auth-delete-output-owner", "DTO042")
	var channelID int64
	if err := db.QueryRowContext(ctx, `
INSERT INTO provider_endpoints (api_type, name, base_url, status)
VALUES ('openai-compatible', 'taskservice-delete-channel', 'https://example.com/taskservice-delete', 1)
RETURNING id`).Scan(&channelID); err != nil {
		t.Fatalf("create channel failed: %v", err)
	}
	account := &accounts.ApiAccount{
		Name:               "taskservice-delete-account",
		ProviderEndpointID: channelID,
		ApiType:            "openai-compatible",
		Models:             []accounts.AccountModel{{Name: "gpt-image-1", Enabled: true}},
		ApiKey:             "test",
		ProxyURL:           "https://example.com/taskservice-delete",
		Status:             1,
	}
	if err := accountRepo.Create(ctx, account); err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	task := &tasks.Task{
		TaskID:                     "delete-with-outputs",
		UserID:                     owner.ID,
		AssignedProviderEndpointID: tasks.IDPtr(account.ProviderEndpointID),
		AccountID:                  tasks.IDPtr(account.ID),
		Status:                     tasks.StatusRunning,
		LeaseToken:                 "lease-delete",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task failed: %v", err)
	}

	taskOutputPath := "uploads/tasks/" + strconv.FormatInt(task.UserID, 10) + "/delete-with-outputs/2026/06/03/result.png"
	rows, err := taskRepo.CompleteWithOutputsAndLeaseToken(ctx, task.TaskID, []string{tasks.StatusRunning}, account.ID, account.ProviderEndpointID, loadTaskServiceCredentialID(t, db, account.ID), task.LeaseToken, `{"status":"success"}`, time.Now(), []tasks.TaskOutput{
		{
			Role:       "image",
			URL:        "/" + taskOutputPath,
			StorageKey: taskOutputPath,
			MIMEType:   "image/png",
			Width:      1024,
			Height:     1024,
		},
	}, nil)
	if err != nil {
		t.Fatalf("complete with outputs failed: %v", err)
	}
	if rows != 1 {
		t.Fatalf("complete rows=%d, want 1", rows)
	}

	cleaner := &recordingTaskOutputCleaner{}
	svc := NewService(ServiceDeps{
		TaskRepo:          taskRepo,
		CancelStore:       &fakeCancelStore{},
		TaskOutputCleaner: cleaner,
	}, ServiceOptions{})

	if err := svc.DeleteUserTask(ctx, task.TaskID, task.UserID); err != nil {
		t.Fatalf("DeleteUserTask failed: %v", err)
	}

	if len(cleaner.calls) != 1 || cleaner.calls[0].userID != task.UserID || cleaner.calls[0].taskID != task.TaskID {
		t.Fatalf("cleaner calls=%+v, want one call for user/task", cleaner.calls)
	}
	outputsByTask, err := taskRepo.ListOutputsByTaskIDs(ctx, []string{task.TaskID})
	if err != nil {
		t.Fatalf("list outputs failed: %v", err)
	}
	if len(outputsByTask[task.TaskID]) != 0 {
		t.Fatalf("outputs after delete=%+v, want none", outputsByTask[task.TaskID])
	}
	var mediaRefCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_refs WHERE task_id = $1 AND ref_type = 'task_output'`, task.TaskID).Scan(&mediaRefCount); err != nil {
		t.Fatalf("count task output media refs failed: %v", err)
	}
	if mediaRefCount != 0 {
		t.Fatalf("task output media refs after delete=%d, want 0", mediaRefCount)
	}
	if _, err := taskRepo.GetByTaskID(ctx, task.TaskID); err == nil {
		t.Fatal("deleted task is still queryable")
	}
}

func TestDeleteUserTaskSucceedsWhenOutputCleanupFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)

	task := &tasks.Task{
		TaskID:     "task_delete_output_fail",
		UserID:     42,
		UserPrompt: "test",
		Prompt:     "test",
		Status:     tasks.StatusCompleted,
		Params:     "{}",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	failingCleaner := &recordingTaskOutputCleaner{err: errors.New("output cleanup failed")}
	svc := NewService(ServiceDeps{
		TaskRepo:          taskRepo,
		CancelStore:       taskqueue.NewPostgresTaskRuntimeStore(db),
		TaskOutputCleaner: failingCleaner,
	}, ServiceOptions{EventHub: NewEventHub()})

	// Should succeed despite output cleanup failure (best-effort)
	if err := svc.DeleteUserTask(ctx, "task_delete_output_fail", 42); err != nil {
		t.Fatalf("DeleteUserTask should succeed even when output cleanup fails: %v", err)
	}

	if len(failingCleaner.calls) == 0 {
		t.Fatal("output cleaner should have been called")
	}

	if _, err := taskRepo.GetByTaskID(ctx, "task_delete_output_fail"); err == nil {
		t.Fatal("task row should be deleted after DeleteUserTask")
	}
}

func TestDeleteTasksCancelsActiveTasksAndDeletesRows(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	runningTask := &tasks.Task{
		TaskID: "admin_delete_running",
		UserID: 42,
		Prompt: "running",
		Status: tasks.StatusRunning,
		Params: "{}",
	}
	completedAt := time.Now()
	completedTask := &tasks.Task{
		TaskID:      "admin_delete_completed",
		UserID:      43,
		Prompt:      "completed",
		Status:      tasks.StatusCompleted,
		Params:      "{}",
		CompletedAt: &completedAt,
	}
	for _, task := range []*tasks.Task{runningTask, completedTask} {
		if err := taskRepo.Create(ctx, task); err != nil {
			t.Fatalf("create task %q: %v", task.TaskID, err)
		}
	}

	cancelStore := &fakeCancelStore{}
	cleaner := &recordingTaskOutputCleaner{}
	svc := NewService(ServiceDeps{
		TaskRepo:          taskRepo,
		CancelStore:       cancelStore,
		TaskOutputCleaner: cleaner,
	}, ServiceOptions{})

	result, err := svc.DeleteTasks(ctx, []string{" " + runningTask.TaskID + " ", completedTask.TaskID, runningTask.TaskID})
	if err != nil {
		t.Fatalf("DeleteTasks failed: %v", err)
	}
	if result.DeletedCount != 2 {
		t.Fatalf("deleted_count=%d, want 2", result.DeletedCount)
	}
	if len(cancelStore.marked) != 1 || cancelStore.marked[0] != runningTask.TaskID {
		t.Fatalf("cancel markers=%+v, want running task only", cancelStore.marked)
	}
	if len(cleaner.calls) != 2 {
		t.Fatalf("cleaner calls=%+v, want both existing tasks", cleaner.calls)
	}
	for _, taskID := range []string{runningTask.TaskID, completedTask.TaskID} {
		if _, err := taskRepo.GetByTaskID(ctx, taskID); err == nil {
			t.Fatalf("task %q should be deleted", taskID)
		}
	}
}

func TestDeleteTasksDoesNotDeleteWhenCancellationMarkerFails(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	taskRepo := taskstore.NewRepository(db)
	task := &tasks.Task{
		TaskID: "admin_delete_marker_failed",
		UserID: 42,
		Prompt: "running",
		Status: tasks.StatusRunning,
		Params: "{}",
	}
	if err := taskRepo.Create(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	markerErr := errors.New("marker failed")
	svc := NewService(ServiceDeps{
		TaskRepo:    taskRepo,
		CancelStore: &fakeCancelStore{markErr: markerErr},
	}, ServiceOptions{})

	if _, err := svc.DeleteTasks(ctx, []string{task.TaskID}); !errors.Is(err, markerErr) {
		t.Fatalf("DeleteTasks error=%v, want marker error", err)
	}
	if _, err := taskRepo.GetByTaskID(ctx, task.TaskID); err != nil {
		t.Fatalf("task should remain after marker failure: %v", err)
	}
}

func TestDeleteTasksRejectsEmptySelection(t *testing.T) {
	svc := NewService(ServiceDeps{}, ServiceOptions{})

	if _, err := svc.DeleteTasks(context.Background(), []string{" "}); !validation.Is(err) {
		t.Fatalf("DeleteTasks error=%v, want validation error", err)
	}
}

func loadTaskServiceCredentialID(t testing.TB, db *sql.DB, accountID int64) int64 {
	t.Helper()
	var credentialID int64
	if err := db.QueryRow(`
SELECT id
FROM api_account_secrets
WHERE account_id = $1 AND status = 'active'
ORDER BY rotated_at DESC, id DESC
LIMIT 1`, accountID).Scan(&credentialID); err != nil {
		t.Fatalf("load account %d credential failed: %v", accountID, err)
	}
	return credentialID
}
