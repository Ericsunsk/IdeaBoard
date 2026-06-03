package accounts

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"api/internal/boards"
	"api/internal/prompts"
	"api/internal/users"
	"api/internal/workflow"
)

func TestTemplateValueDescriptions(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "viewpoint uses display label",
			got:  prompts.DescribeViewpointTemplateValue("top_45"),
			want: "45° 侧视",
		},
		{
			name: "lighting uses readable label",
			got:  prompts.DescribeViewpointLightingTemplateValue("hard_light"),
			want: "质感硬光",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("got %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestParseWorkflowFeatureSlug(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{name: "canonical slug is valid", raw: "style-fusion", want: "style-fusion", ok: true},
		{name: "remove-bg slug is valid", raw: "remove-bg", want: "remove-bg", ok: true},
		{name: "underscore slug is invalid", raw: "style_fusion", want: "", ok: false},
		{name: "unsupported slug is invalid", raw: "line-draft", want: "", ok: false},
		{name: "invalid chars are rejected", raw: "style fusion", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := workflow.ParseWorkflowFeatureSlug(tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("raw=%q got=(%q,%v), want=(%q,%v)", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestApplyFeatureSystemPromptUsesCanonicalKeys(t *testing.T) {
	params := map[string]interface{}{
		"upscale_mode": "enhancement",
	}

	got := prompts.ApplyFeatureSystemPrompt(
		"base prompt",
		"user prompt",
		"hd-upscale",
		params,
		func(slug string) (string, error) {
			return "SYS={{user_prompt}}|{{upscale_mode}}", nil
		},
	)

	want := "SYS=user prompt|enhancement"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFeatureSystemPromptDoesNotContainHiddenConstraintSnippet(t *testing.T) {
	got := prompts.ApplyFeatureSystemPrompt(
		"user prompt",
		"user prompt",
		"image-extend",
		map[string]interface{}{
			"extend_top": 30,
		},
		func(slug string) (string, error) {
			return "SYS=canvas extend only", nil
		},
	)

	if got != "SYS=canvas extend only user prompt" {
		t.Fatalf("got %q, want only configured system prompt + base prompt", got)
	}
	if strings.Contains(got, "【执行语义】") {
		t.Fatalf("expected no hidden constraint text, got %q", got)
	}
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
				"image_weights": []interface{}{0.6, 0.4},
			},
			template: "SYS {{user_prompt}} weights={{image_weights}}",
			want:     "SYS 融合但不要太夸张 weights=Image 1: 60%, Image 2: 40%",
		},
		{
			name:       "canvas extend",
			feature:    workflow.FeatureSlugImageExtend,
			basePrompt: "user prompt",
			userPrompt: "右侧延展为工作室背景",
			params: map[string]interface{}{
				"extend_top":    0,
				"extend_right":  22,
				"extend_bottom": 0,
				"extend_left":   0,
			},
			template: "SYS {{user_prompt}} top={{extend_top}} right={{extend_right}} bottom={{extend_bottom}} left={{extend_left}}",
			want:     "SYS 右侧延展为工作室背景 top=0 right=22 bottom=0 left=0",
		},
		{
			name:       "hd-upscale",
			feature:    workflow.FeatureSlugHDUpscale,
			basePrompt: "user prompt",
			userPrompt: "",
			params: map[string]interface{}{
				"original_image_url": "https://example.com/original.png",
				"upscale_mode":       "enhancement",
				"upscale_factor":     2,
			},
			template: "SYS mode={{upscale_mode}} factor={{upscale_factor}}",
			want:     "SYS mode=enhancement factor=2",
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

func TestImageExtendEmptyUserPromptCanStillBuildExecutablePrompt(t *testing.T) {
	fullPrompt := applyTaskSystemPrompt(
		"",
		"",
		workflow.FeatureSlugImageExtend,
		map[string]interface{}{
			"extend_right": 30,
		},
		func(string) (string, error) {
			return "SYS 扩图 right={{extend_right}}", nil
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
		if err := validateSubmitTaskMediaRef("/uploads/workspace/OWNR1A/board-1/content/a.png"); err != nil {
			t.Fatalf("expected structured upload path to pass: %v", err)
		}
	})

	t.Run("reject legacy generic upload path", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("/uploads/2026/05/13/a.png"); err == nil {
			t.Fatal("expected legacy generic upload path to fail")
		}
	})

	t.Run("reject non upload relative path", func(t *testing.T) {
		if err := validateSubmitTaskMediaRef("/tmp/a.png"); err == nil {
			t.Fatal("expected non upload relative path to fail")
		}
	})

	t.Run("reject oversized data url", func(t *testing.T) {
		tooLarge := "data:image/png;base64," + strings.Repeat("a", maxSubmitTaskMediaDataURLBytes)
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

func TestSubmitTaskWorkspaceUploadRefsRequireBoardAccess(t *testing.T) {
	ctx := context.Background()
	db := newAccountsWorkspaceUploadTestDB(t)
	userRepo := users.NewUserRepository(db)
	owner := mustCreateAccountsUploadUser(t, userRepo, "owner", "owner-auth", "OWNR1A")
	member := mustCreateAccountsUploadUser(t, userRepo, "member", "member-auth", "MEMB1A")
	outsider := mustCreateAccountsUploadUser(t, userRepo, "outsider", "outsider-auth", "OUTS1A")
	boardRepo := boards.NewBoardRepository(db)
	now := time.Now()
	if err := boardRepo.Upsert(ctx, &boards.UserBoard{BoardID: "board-1", UserID: owner.ID, Name: "Demo", State: "{}", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create board failed: %v", err)
	}
	if err := boardRepo.UpsertMember(ctx, owner.ID, "board-1", member.ID, boards.BoardAccessRoleViewer); err != nil {
		t.Fatalf("create board member failed: %v", err)
	}

	svc := NewAccountService(AccountServiceDeps{
		UploadBoards: testAccountUploadBoardAccessResolver{service: boards.NewBoardService(boardRepo, userRepo)},
		UploadOwners: testAccountUploadOwnerStorageUIDResolver{repo: userRepo},
	}, AccountServiceOptions{})
	allowed := "/uploads/workspace/OWNR1A/board-1/content/demo.png"
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, allowed); err != nil {
		t.Fatalf("expected member to use workspace upload: %v", err)
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, outsider.ID, allowed); err == nil {
		t.Fatalf("expected outsider to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/workspace/WRONG1/board-1/content/demo.png"); err == nil {
		t.Fatalf("expected owner segment mismatch to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/2026/05/13/demo.png"); err == nil {
		t.Fatalf("expected legacy generic upload path to be rejected")
	}
	if err := svc.validateSubmitTaskMediaRef(ctx, member.ID, "/uploads/profile/MEMB1A/avatar/demo.png"); err == nil {
		t.Fatalf("expected non-workspace upload path to be rejected")
	}
}

type testAccountUploadBoardAccessResolver struct {
	service *boards.BoardService
}

func (r testAccountUploadBoardAccessResolver) ResolveUploadBoardAccess(ctx context.Context, userID int64, boardID string) (int64, error) {
	board, _, err := r.service.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return 0, err
	}
	return board.UserID, nil
}

type testAccountUploadOwnerStorageUIDResolver struct {
	repo *users.UserRepository
}

func (r testAccountUploadOwnerStorageUIDResolver) GetStorageUIDByUserID(ctx context.Context, userID int64) (string, error) {
	user, err := r.repo.GetByID(ctx, userID)
	if err != nil {
		return "", err
	}
	return user.StorageUID, nil
}

func newAccountsWorkspaceUploadTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return newAccountsTestDB(t)
}

func mustCreateAccountsUploadUser(t *testing.T, repo *users.UserRepository, username string, authUserID string, storageUID string) *users.User {
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
