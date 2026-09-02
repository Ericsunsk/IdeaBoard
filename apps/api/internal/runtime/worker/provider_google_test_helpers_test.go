package worker

import (
	"context"
	"time"

	"ideaboard/api/internal/accountpool/accounts"
	"ideaboard/api/internal/workflow"
)

func (c *ProviderExecutor) callGoogleAIWithRequest(ctx context.Context, account *accounts.ApiAccount, providerReq providerRequest, timeout time.Duration) (providerCallResult, error) {
	return c.callGoogleProviderWithRequest(ctx, account, workflow.ProviderGoogleAI, providerReq, timeout)
}

func (c *ProviderExecutor) callGoogleGeminiGenerateContent(ctx context.Context, account *accounts.ApiAccount, baseURL string, providerReq providerRequest, timeout time.Duration, googleParams googleProviderParams) (providerCallResult, error) {
	return c.callGoogleGeminiGenerateContentForAPIType(ctx, account, workflow.ProviderGoogleAI, baseURL, providerReq, timeout, googleParams)
}
