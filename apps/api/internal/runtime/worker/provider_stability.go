package worker

import (
	"api/internal/platform/outbound"
	"api/internal/platform/safehttp"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"api/internal/accounts"
)

func (c *ProviderExecutor) callStabilityAI(ctx context.Context, account *accounts.ApiAccount, prompt string, width, height int, timeout time.Duration, resolvedModel string) (providerCallResult, error) {
	baseURL := "https://api.stability.ai"
	if account.ProxyURL != "" {
		baseURL = account.ProxyURL
	}
	validatedBaseURL, validateErr := outbound.ValidateConfiguredEndpoint(baseURL, outbound.AllowHostsFromEnv()...)
	if validateErr != nil {
		return providerCallResult{}, validateErr
	}
	baseURL = validatedBaseURL

	engine := "stable-diffusion-xl-1024-v1-0"
	if strings.TrimSpace(resolvedModel) != "" {
		engine = strings.TrimSpace(resolvedModel)
	}

	reqBody := map[string]interface{}{
		"text_prompts": []map[string]interface{}{
			{"text": prompt, "weight": 1},
		},
		"width":   width,
		"height":  height,
		"samples": 1,
	}
	bodyJSON, _ := json.Marshal(reqBody)

	url := fmt.Sprintf("%s/v1/generation/%s/text-to-image", baseURL, engine)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(bodyJSON))
	if err != nil {
		return providerCallResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+account.ApiKey)
	req.Header.Set("Accept", "application/json")

	log.Printf("[Worker] StabilityAI 请求: engine=%s, %dx%d", engine, width, height)

	resp, err := safehttp.DoRequestWithAllowHosts(ctx, req, timeout, outbound.AllowHostsFromEnv()...)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readLimitedBody(resp.Body)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return providerCallResult{}, err
		}
		return providerCallResult{}, fmt.Errorf("读取 API 响应失败: %w", err)
	}

	if resp.StatusCode != 200 {
		return providerCallResult{}, newUpstreamHTTPError(resp.StatusCode, truncateErrorBody(string(respBody)))
	}

	var respData struct {
		Artifacts []struct {
			Base64 string `json:"base64"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		return providerCallResult{}, fmt.Errorf("解析响应失败: %v", err)
	}
	if len(respData.Artifacts) == 0 {
		return providerCallResult{}, fmt.Errorf("API 未返回图片")
	}

	// 返回 base64 数据 URL
	return providerCallResult{
		ImageURL: "data:image/png;base64," + respData.Artifacts[0].Base64,
		Model:    engine,
	}, nil
}
