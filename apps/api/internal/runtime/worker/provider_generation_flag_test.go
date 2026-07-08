package worker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/platform/settings"
	"ideaboard/api/internal/tasking/taskqueue"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

type mutableGoogleNativeGenerationSettings struct {
	setting settings.GoogleNativeGenerationSetting
	err     error
}

func (p *mutableGoogleNativeGenerationSettings) GetGoogleNativeGeneration(context.Context) (settings.GoogleNativeGenerationSetting, error) {
	if p.err != nil {
		return settings.GoogleNativeGenerationSetting{}, p.err
	}
	return p.setting, nil
}

func TestProviderExecutorGoogleNativeGenerationFlagBlocksBeforeUpstream(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}]}`))
	}))
	defer server.Close()

	account := googleNativeFlagTestAccount(server.URL)
	task := googleNativeFlagTestTask()
	executor := &ProviderExecutor{
		generationSettings: &mutableGoogleNativeGenerationSettings{
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeDisabled,
				CanaryPercent: 100,
			},
		},
	}

	_, err := executor.callDrawingAPIWithAssignment(context.Background(), account, task, googleNativeFlagTestAssignment(server.URL))
	if err == nil {
		t.Fatalf("expected disabled google native generation to fail")
	}
	stage, category, ok := executionFailureDetails(err)
	if !ok || stage != tasks.ExecutionStageAssigned || category != tasks.FailureCategoryInputInvalid {
		t.Fatalf("failure details = stage=%q category=%q ok=%v", stage, category, ok)
	}
	if !strings.Contains(err.Error(), "Google AI 原生生成通道当前已关闭") {
		t.Fatalf("error = %q, want disabled message", err.Error())
	}
	if hits != 0 {
		t.Fatalf("upstream hits = %d, want 0", hits)
	}
}

func TestProviderExecutorGoogleNativeGenerationFlagCanaryExcludedBlocksBeforeUpstream(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}]}`))
	}))
	defer server.Close()

	executor := &ProviderExecutor{
		generationSettings: &mutableGoogleNativeGenerationSettings{
			setting: settings.GoogleNativeGenerationSetting{
				Mode:          settings.GoogleNativeGenerationModeCanary,
				CanaryPercent: 0,
			},
		},
	}

	_, err := executor.callDrawingAPIWithAssignment(context.Background(), googleNativeFlagTestAccount(server.URL), googleNativeFlagTestTask(), googleNativeFlagTestAssignment(server.URL))
	if err == nil {
		t.Fatalf("expected canary-excluded google native generation to fail")
	}
	if !strings.Contains(err.Error(), "Google AI 原生生成通道当前处于灰度开放") {
		t.Fatalf("error = %q, want canary exclusion message", err.Error())
	}
	if hits != 0 {
		t.Fatalf("upstream hits = %d, want 0", hits)
	}
}

func TestProviderExecutorGoogleNativeGenerationFlagRollbackStopsQueuedWork(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"aGVsbG8="}}]}}]}`))
	}))
	defer server.Close()

	generationSettings := &mutableGoogleNativeGenerationSettings{
		setting: settings.GoogleNativeGenerationSetting{
			Mode:          settings.GoogleNativeGenerationModeEnabled,
			CanaryPercent: 0,
		},
	}
	executor := &ProviderExecutor{generationSettings: generationSettings}

	_, firstErr := executor.callDrawingAPIWithAssignment(context.Background(), googleNativeFlagTestAccount(server.URL), googleNativeFlagTestTask(), googleNativeFlagTestAssignment(server.URL))
	if firstErr == nil {
		t.Fatalf("expected first call to fail later because result materializer is not configured")
	}
	if hits != 1 {
		t.Fatalf("upstream hits after enabled call = %d, want 1", hits)
	}

	generationSettings.setting = settings.GoogleNativeGenerationSetting{
		Mode:          settings.GoogleNativeGenerationModeDisabled,
		CanaryPercent: 0,
	}
	_, secondErr := executor.callDrawingAPIWithAssignment(context.Background(), googleNativeFlagTestAccount(server.URL), googleNativeFlagTestTask(), googleNativeFlagTestAssignment(server.URL))
	if secondErr == nil {
		t.Fatalf("expected second call to fail after rollback")
	}
	if hits != 1 {
		t.Fatalf("upstream hits after rollback = %d, want unchanged 1", hits)
	}
}

func TestProviderExecutorGoogleNativeGenerationFlagReadFailureBlocksBeforeUpstream(t *testing.T) {
	var hits int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	executor := &ProviderExecutor{
		generationSettings: &mutableGoogleNativeGenerationSettings{err: errors.New("settings unavailable")},
	}
	_, err := executor.callDrawingAPIWithAssignment(context.Background(), googleNativeFlagTestAccount(server.URL), googleNativeFlagTestTask(), googleNativeFlagTestAssignment(server.URL))
	if err == nil || !strings.Contains(err.Error(), "生成策略设置读取失败") {
		t.Fatalf("error = %v, want settings read failure", err)
	}
	if hits != 0 {
		t.Fatalf("upstream hits = %d, want 0", hits)
	}
}

func googleNativeFlagTestAccount(proxyURL string) *accounts.ApiAccount {
	return &accounts.ApiAccount{
		ID:                 101,
		Name:               "google-native",
		ProviderEndpointID: 501,
		ApiType:            workflow.ProviderGoogleAI,
		ProxyURL:           proxyURL,
		ApiKey:             "google-key",
		Timeout:            int((2 * time.Second).Seconds()),
		Models:             []accounts.AccountModel{{Name: "gemini-2.5-flash-image", Enabled: true}},
	}
}

func googleNativeFlagTestTask() *tasks.TaskPayload {
	params, _ := json.Marshal(map[string]interface{}{
		"model": "gemini-2.5-flash-image",
	})
	return &tasks.TaskPayload{
		TaskID:     "task-google-native-flag",
		UserID:     42,
		Prompt:     "draw a product mockup",
		Params:     string(params),
		AccountIDs: []int64{101},
		CreatedAt:  time.Now().Unix(),
	}
}

func googleNativeFlagTestAssignment(proxyURL string) taskqueue.TaskAssignment {
	return taskqueue.TaskAssignment{
		TaskID:                  "task-google-native-flag",
		AccountID:               101,
		ProviderEndpointID:      501,
		CredentialID:            701,
		ResolvedModel:           "gemini-2.5-flash-image",
		APIType:                 workflow.ProviderGoogleAI,
		ProviderEndpointName:    "google-native",
		ProviderEndpointBaseURL: proxyURL,
		LeaseToken:              "lease-token",
	}
}
