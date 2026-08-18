package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/identity/auth/internalcontract"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/identity/trust"
	"ideaboard/api/internal/platform/config"
)

var (
	ErrSessionUnauthorized         = errors.New("auth session unauthorized")
	ErrAuthUserNotFound            = autherrors.ErrAuthUserNotFound
	errAuthServiceResponseTooLarge = errors.New("auth service response body exceeds 2 MiB")
)

const maxAuthServiceResponseBytes int64 = 2 * 1024 * 1024

type SessionIdentity = trust.SessionIdentity

type ProvisionedAuthUser struct {
	AuthUserID            string `json:"auth_user_id"`
	Email                 string `json:"email"`
	DisplayName           string `json:"display_name"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
	Role                  string `json:"role"`
	Existing              bool   `json:"existing"`
}

type ServiceClient struct {
	contract    *internalcontract.ClientWithResponses
	httpClient  *http.Client
	baseURL     string
	internalKey string
	initErr     error
}

func NewServiceClient(cfg config.AuthServiceConfig) *ServiceClient {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 10
	transport.IdleConnTimeout = 90 * time.Second

	internalKey := strings.TrimSpace(cfg.InternalAPIKey)
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &boundedResponseTransport{
			base:     transport,
			maxBytes: maxAuthServiceResponseBytes,
		},
	}
	client, err := internalcontract.NewClientWithResponses(
		strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		internalcontract.WithHTTPClient(httpClient),
		internalcontract.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			if internalKey != "" {
				req.Header.Set("X-Internal-Key", internalKey)
			}
			return nil
		}),
	)
	return &ServiceClient{
		contract:    client,
		httpClient:  httpClient,
		baseURL:     strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		internalKey: internalKey,
		initErr:     err,
	}
}

type MailConfigSummary struct {
	Configured        bool    `json:"configured"`
	APIKeyConfigured  bool    `json:"api_key_configured"`
	FromName          string  `json:"from_name"`
	FromEmail         string  `json:"from_email"`
	ReplyTo           string  `json:"reply_to"`
	ConfigVersion     int     `json:"config_version"`
	Readiness         string  `json:"readiness"`
	UpdatedAt         *string `json:"updated_at"`
	LastTestedAt      *string `json:"last_tested_at"`
	LastTestErrorCode *string `json:"last_test_error_code"`
}

type MailConfigUpdate struct {
	APIKey    string
	FromName  string
	FromEmail string
	ReplyTo   string
}

type SocialProviderSettingsSummary struct {
	Provider               string   `json:"provider"`
	Enabled                bool     `json:"enabled"`
	Configured             bool     `json:"configured"`
	Available              bool     `json:"available"`
	ClientID               string   `json:"client_id"`
	ClientSecretConfigured bool     `json:"client_secret_configured"`
	Scopes                 []string `json:"scopes"`
	ConfigVersion          int      `json:"config_version"`
	CallbackURL            string   `json:"callback_url"`
	UpdatedAt              *string  `json:"updated_at"`
}

type SocialProviderSettingsUpdate struct {
	Enabled      bool
	ClientID     string
	ClientSecret string
}

type SocialProviderCapabilities struct {
	Social map[string]bool `json:"social"`
}

type SocialProviderSettingsError struct {
	Status  int
	Message string
}

type ImpersonationError struct {
	Status  int
	Message string
}

func (e *ImpersonationError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return "auth impersonation request failed"
	}
	return e.Message
}

func (e *ImpersonationError) StatusCode() int {
	if e == nil || e.Status <= 0 {
		return http.StatusBadGateway
	}
	return e.Status
}

func (e *SocialProviderSettingsError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return "auth social provider settings request failed"
	}
	return e.Message
}

// MailConfigError preserves the Auth service status while keeping provider details
// behind the Auth boundary. The Admin handler maps only these stable statuses.
type MailConfigError struct {
	Status  int
	Message string
}

func (e *MailConfigError) Error() string {
	if strings.TrimSpace(e.Message) == "" {
		return "auth mail configuration request failed"
	}
	return e.Message
}

func (c *ServiceClient) GetMailConfig(ctx context.Context) (*MailConfigSummary, error) {
	var summary MailConfigSummary
	if err := c.mailConfigRequest(ctx, http.MethodGet, "/internal/mail/config", nil, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *ServiceClient) UpdateMailConfig(ctx context.Context, input MailConfigUpdate, actorID int64) (*MailConfigSummary, error) {
	var summary MailConfigSummary
	body := map[string]any{
		"from_name":  input.FromName,
		"from_email": input.FromEmail,
		"actor_id":   actorID,
	}
	if strings.TrimSpace(input.APIKey) != "" {
		body["api_key"] = input.APIKey
	}
	if strings.TrimSpace(input.ReplyTo) != "" {
		body["reply_to"] = input.ReplyTo
	}
	if err := c.mailConfigRequest(ctx, http.MethodPut, "/internal/mail/config", body, &summary); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *ServiceClient) ClearMailConfig(ctx context.Context, actorID int64) error {
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	return c.mailConfigRequest(ctx, http.MethodDelete, "/internal/mail/config", map[string]any{"actor_id": actorID}, &envelope)
}

func (c *ServiceClient) SendMailConfigTest(ctx context.Context, recipient string, actorID int64) error {
	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	return c.mailConfigRequest(ctx, http.MethodPost, "/internal/mail/config/test", map[string]any{"recipient": recipient, "actor_id": actorID}, &envelope)
}

func (c *ServiceClient) GetSocialProviderSettings(ctx context.Context, provider string) (*SocialProviderSettingsSummary, error) {
	var summary SocialProviderSettingsSummary
	path := "/internal/social-providers/" + url.PathEscape(strings.TrimSpace(provider))
	if err := c.socialProviderRequest(ctx, http.MethodGet, path, nil, &summary, true); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *ServiceClient) UpdateSocialProviderSettings(ctx context.Context, provider string, input SocialProviderSettingsUpdate) (*SocialProviderSettingsSummary, error) {
	body := map[string]any{
		"enabled":   input.Enabled,
		"client_id": input.ClientID,
	}
	if strings.TrimSpace(input.ClientSecret) != "" {
		body["client_secret"] = input.ClientSecret
	}
	var summary SocialProviderSettingsSummary
	path := "/internal/social-providers/" + url.PathEscape(strings.TrimSpace(provider))
	if err := c.socialProviderRequest(ctx, http.MethodPut, path, body, &summary, true); err != nil {
		return nil, err
	}
	return &summary, nil
}

func (c *ServiceClient) GetSocialProviderCapabilities(ctx context.Context) (*SocialProviderCapabilities, error) {
	var capabilities SocialProviderCapabilities
	if err := c.socialProviderRequest(ctx, http.MethodGet, "/api/auth/capabilities", nil, &capabilities, false); err != nil {
		return nil, err
	}
	return &capabilities, nil
}

// ImpersonateUser bridges the Admin API session to Better Auth's native
// impersonation endpoint. The browser Cookie is forwarded; the internal API
// key is intentionally not sent because this is a public-session route.
func (c *ServiceClient) ImpersonateUser(ctx context.Context, cookieHeader string, authUserID string) ([]string, error) {
	if c == nil || c.httpClient == nil || strings.TrimSpace(c.baseURL) == "" {
		return nil, &ImpersonationError{Status: http.StatusBadGateway, Message: "认证服务未配置"}
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return nil, &ImpersonationError{Status: http.StatusBadRequest, Message: "认证用户 ID 不能为空"}
	}
	body, err := json.Marshal(map[string]string{"userId": normalizedID})
	if err != nil {
		return nil, fmt.Errorf("encode auth impersonation request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/auth/admin/impersonate-user", strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build auth impersonation request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	if cookie := strings.TrimSpace(cookieHeader); cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	upstreamResponse, err := c.httpClient.Do(request)
	if err != nil {
		return nil, &ImpersonationError{Status: http.StatusBadGateway, Message: "认证服务不可用"}
	}
	defer upstreamResponse.Body.Close()
	if _, err := io.Copy(io.Discard, upstreamResponse.Body); err != nil {
		return nil, &ImpersonationError{Status: http.StatusBadGateway, Message: "读取认证服务响应失败"}
	}
	if upstreamResponse.StatusCode < http.StatusOK || upstreamResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, &ImpersonationError{Status: upstreamResponse.StatusCode, Message: "认证服务拒绝伪装登录"}
	}
	cookies := upstreamResponse.Header.Values("Set-Cookie")
	for _, cookie := range cookies {
		if strings.TrimSpace(cookie) != "" {
			return append([]string(nil), cookies...), nil
		}
	}
	return nil, &ImpersonationError{Status: http.StatusBadGateway, Message: "认证服务未返回伪装 Session"}
}

func (c *ServiceClient) socialProviderRequest(ctx context.Context, method, path string, body any, result any, includeInternalKey bool) error {
	if _, err := c.contractClient(); err != nil {
		return err
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("auth service base URL is not configured")
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode auth social provider request: %w", err)
		}
		reader = strings.NewReader(string(payload))
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build auth social provider request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if includeInternalKey && c.internalKey != "" {
		request.Header.Set("X-Internal-Key", c.internalKey)
	}
	upstreamResponse, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call auth social provider service: %w", err)
	}
	defer upstreamResponse.Body.Close()
	payload, err := io.ReadAll(upstreamResponse.Body)
	if err != nil {
		return fmt.Errorf("read auth social provider response: %w", err)
	}
	var envelope struct {
		Code *int            `json:"code"`
		Msg  *string         `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &SocialProviderSettingsError{Status: http.StatusBadGateway, Message: "认证服务返回无效响应"}
	}
	if upstreamResponse.StatusCode < 200 || upstreamResponse.StatusCode >= 300 || envelope.Code == nil || envelope.Msg == nil || *envelope.Code != 0 {
		message := "OAuth Provider 配置操作失败"
		if envelope.Msg != nil && strings.TrimSpace(*envelope.Msg) != "" {
			message = strings.TrimSpace(*envelope.Msg)
		}
		return &SocialProviderSettingsError{Status: upstreamResponse.StatusCode, Message: message}
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return &SocialProviderSettingsError{Status: http.StatusBadGateway, Message: "认证服务返回无效响应"}
	}
	if err := json.Unmarshal(envelope.Data, result); err != nil {
		return &SocialProviderSettingsError{Status: http.StatusBadGateway, Message: "认证服务返回无效响应"}
	}
	return nil
}

func (c *ServiceClient) mailConfigRequest(ctx context.Context, method, path string, body any, result any) error {
	if _, err := c.contractClient(); err != nil {
		return err
	}
	if strings.TrimSpace(c.baseURL) == "" {
		return fmt.Errorf("auth service base URL is not configured")
	}
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode auth mail configuration request: %w", err)
		}
		reader = strings.NewReader(string(payload))
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build auth mail configuration request: %w", err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.internalKey != "" {
		request.Header.Set("X-Internal-Key", c.internalKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("call auth mail configuration: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read auth mail configuration response: %w", err)
	}
	var envelope struct {
		Code *int            `json:"code"`
		Msg  *string         `json:"msg"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(payload, &envelope); err != nil {
		return &MailConfigError{Status: response.StatusCode, Message: "认证服务返回无效响应"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 || envelope.Code == nil || envelope.Msg == nil || *envelope.Code != 0 {
		message := "邮件配置操作失败"
		if envelope.Msg != nil && strings.TrimSpace(*envelope.Msg) != "" {
			message = strings.TrimSpace(*envelope.Msg)
		}
		return &MailConfigError{Status: response.StatusCode, Message: message}
	}
	if result != nil && (len(envelope.Data) == 0 || string(envelope.Data) == "null") {
		return &MailConfigError{Status: http.StatusBadGateway, Message: "认证服务返回无效响应"}
	}
	if result != nil {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			return fmt.Errorf("decode auth mail configuration response: %w", err)
		}
	}
	return nil
}

func (c *ServiceClient) ValidateSession(ctx context.Context, cookieHeader string) (*SessionIdentity, error) {
	client, err := c.contractClient()
	if err != nil {
		return nil, err
	}
	response, err := client.ValidateSessionWithResponse(ctx, withCookieHeader(cookieHeader))
	if err != nil {
		return nil, err
	}
	if response.StatusCode() == http.StatusUnauthorized {
		return nil, ErrSessionUnauthorized
	}
	if response.StatusCode() != http.StatusOK {
		return nil, contractFailure("session check", response.StatusCode(), response.Body)
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("auth service session check returned an invalid success envelope")
	}
	if _, err := requireSuccessEnvelopeFields(response.Body); err != nil {
		return nil, fmt.Errorf("auth service session check returned an invalid success envelope: %w", err)
	}
	if response.JSON200.Code != 0 {
		if int(response.JSON200.Code) == http.StatusUnauthorized {
			return nil, ErrSessionUnauthorized
		}
		return nil, fmt.Errorf("auth service session rejected: %s", nonEmpty(response.JSON200.Msg, "unknown error"))
	}
	contractSession := response.JSON200.Data.Session
	avatar := ""
	if contractSession.Avatar != nil {
		avatar = *contractSession.Avatar
	}
	session, trustErr := trust.ValidateSessionIdentity(trust.SessionIdentity{
		AuthUserID:            contractSession.AuthUserID,
		Email:                 contractSession.Email,
		DisplayName:           contractSession.DisplayName,
		Avatar:                avatar,
		ClaimIdentity:         contractSession.ClaimIdentity,
		ClaimIdentityKind:     string(contractSession.ClaimIdentityKind),
		ClaimIdentityVerified: contractSession.ClaimIdentityVerified,
	})
	if trustErr != nil {
		return nil, fmt.Errorf("%w: auth service returned an untrusted session identity: %v", ErrSessionUnauthorized, trustErr)
	}
	return &session, nil
}

func (c *ServiceClient) EnsureUser(ctx context.Context, email, displayName, password string, allowExisting bool, role ...string) (*ProvisionedAuthUser, error) {
	client, err := c.contractClient()
	if err != nil {
		return nil, err
	}
	response, err := client.EnsureUserWithResponse(ctx, internalcontract.EnsureUserJSONRequestBody{
		AllowExisting: allowExisting,
		DisplayName:   strings.TrimSpace(displayName),
		Email:         strings.ToLower(strings.TrimSpace(email)),
		Password:      password,
		Role:          normalizeEnsureRole(role...),
	})
	if err != nil {
		return nil, err
	}
	if response.StatusCode() != http.StatusOK {
		return nil, contractFailure("ensure user", response.StatusCode(), response.Body)
	}
	if response.JSON200 == nil {
		return nil, fmt.Errorf("auth service ensure user returned an invalid success envelope")
	}
	if _, err := requireSuccessEnvelopeFields(response.Body); err != nil {
		return nil, fmt.Errorf("auth service ensure user returned an invalid success envelope: %w", err)
	}
	if response.JSON200.Code != 0 {
		return nil, fmt.Errorf("auth service ensure user rejected: %s", nonEmpty(response.JSON200.Msg, "unknown error"))
	}
	contractUser := response.JSON200.Data.User
	user := ProvisionedAuthUser{
		AuthUserID:            contractUser.AuthUserID,
		Email:                 contractUser.Email,
		DisplayName:           contractUser.DisplayName,
		ClaimIdentity:         contractUser.ClaimIdentity,
		ClaimIdentityKind:     string(contractUser.ClaimIdentityKind),
		ClaimIdentityVerified: bool(contractUser.ClaimIdentityVerified),
		Role:                  string(contractUser.Role),
		Existing:              contractUser.Existing,
	}
	normalized, trustErr := trust.NormalizeProvisioningIdentity(trust.ProvisioningIdentity{
		AuthUserID:            user.AuthUserID,
		Email:                 user.Email,
		DisplayName:           user.DisplayName,
		ClaimIdentity:         user.ClaimIdentity,
		ClaimIdentityKind:     user.ClaimIdentityKind,
		ClaimIdentityVerified: user.ClaimIdentityVerified,
		Role:                  user.Role,
	})
	if trustErr != nil {
		return nil, fmt.Errorf("auth service returned an untrusted claim identity: %w", trustErr)
	}
	user.AuthUserID = normalized.AuthUserID
	user.Email = normalized.Email
	user.DisplayName = normalized.DisplayName
	user.ClaimIdentity = normalized.ClaimIdentity
	user.ClaimIdentityKind = normalized.ClaimIdentityKind
	user.ClaimIdentityVerified = normalized.ClaimIdentityVerified
	user.Role = normalized.Role
	return &user, nil
}

// GetUserEmails resolves a bounded batch of Auth-owned email identities.
func (c *ServiceClient) GetUserEmails(ctx context.Context, authUserIDs []string) (map[string]string, error) {
	client, err := c.contractClient()
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(authUserIDs))
	seen := make(map[string]struct{}, len(authUserIDs))
	for _, rawID := range authUserIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	response, err := client.GetUserEmailsWithResponse(ctx, internalcontract.GetUserEmailsJSONRequestBody{UserIds: ids})
	if err != nil {
		return nil, err
	}
	if response.StatusCode() != http.StatusOK {
		return nil, contractFailure("get user emails", response.StatusCode(), response.Body)
	}
	if response.JSON200 == nil || response.JSON200.Code != internalcontract.GetUserEmailsEnvelopeCodeN0 {
		return nil, fmt.Errorf("auth service get user emails returned an invalid success envelope")
	}
	emails := make(map[string]string, len(response.JSON200.Data.Users))
	for _, user := range response.JSON200.Data.Users {
		id := strings.TrimSpace(user.AuthUserID)
		email := strings.TrimSpace(user.Email)
		if id != "" && email != "" {
			emails[id] = email
		}
	}
	return emails, nil
}

func (c *ServiceClient) DeleteUser(ctx context.Context, authUserID string) error {
	client, normalizedID, err := c.managedUserClient(authUserID)
	if err != nil {
		return err
	}
	response, err := client.DeleteUserWithResponse(ctx, normalizedID)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		_, err := adminMutationResult("delete user", response.StatusCode(), response.Body, 0, "", false)
		return err
	}
	dataFields, err := adminMutationResult(
		"delete user", response.StatusCode(), response.Body,
		int(response.JSON200.Code), response.JSON200.Msg, true,
	)
	if err != nil {
		return err
	}
	data := response.JSON200.Data
	return validateMutationSuccess(
		"delete user", normalizedID, data.AuthUserID,
		dataFields, "deleted", bool(data.Deleted),
	)
}

func (c *ServiceClient) SetUserRole(ctx context.Context, authUserID string, role string) error {
	client, normalizedID, err := c.managedUserClient(authUserID)
	if err != nil {
		return err
	}
	normalizedRole := normalizeEnsureRole(role)
	response, err := client.SetUserRoleWithResponse(ctx, normalizedID, internalcontract.SetUserRoleJSONRequestBody{
		Role: normalizedRole,
	})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		_, err := adminMutationResult("set user role", response.StatusCode(), response.Body, 0, "", false)
		return err
	}
	dataFields, err := adminMutationResult(
		"set user role", response.StatusCode(), response.Body,
		int(response.JSON200.Code), response.JSON200.Msg, true,
	)
	if err != nil {
		return err
	}
	data := response.JSON200.Data
	return validateMutationSuccess(
		"set user role", normalizedID, data.AuthUserID,
		dataFields, "role", data.Role == normalizedRole,
	)
}

func (c *ServiceClient) BanUser(ctx context.Context, authUserID string, reason string, expiresInSeconds int) error {
	client, normalizedID, err := c.managedUserClient(authUserID)
	if err != nil {
		return err
	}
	body := internalcontract.BanUserJSONRequestBody{UserID: normalizedID}
	if reason := strings.TrimSpace(reason); reason != "" {
		body.BanReason = &reason
	}
	if expiresInSeconds > 0 {
		body.BanExpiresIn = &expiresInSeconds
	}
	response, err := client.BanUserWithResponse(ctx, body)
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		_, err := adminMutationResult("ban", response.StatusCode(), response.Body, 0, "", false)
		return err
	}
	dataFields, err := adminMutationResult(
		"ban", response.StatusCode(), response.Body,
		int(response.JSON200.Code), response.JSON200.Msg, true,
	)
	if err != nil {
		return err
	}
	data := response.JSON200.Data
	return validateMutationSuccess(
		"ban", normalizedID, data.AuthUserID,
		dataFields, "banned", bool(data.Banned),
	)
}

func (c *ServiceClient) UnbanUser(ctx context.Context, authUserID string) error {
	client, normalizedID, err := c.managedUserClient(authUserID)
	if err != nil {
		return err
	}
	response, err := client.UnbanUserWithResponse(ctx, internalcontract.UnbanUserJSONRequestBody{UserID: normalizedID})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		_, err := adminMutationResult("unban", response.StatusCode(), response.Body, 0, "", false)
		return err
	}
	dataFields, err := adminMutationResult(
		"unban", response.StatusCode(), response.Body,
		int(response.JSON200.Code), response.JSON200.Msg, true,
	)
	if err != nil {
		return err
	}
	data := response.JSON200.Data
	return validateMutationSuccess(
		"unban", normalizedID, data.AuthUserID,
		dataFields, "banned", !bool(data.Banned),
	)
}

func (c *ServiceClient) RevokeUserSessions(ctx context.Context, authUserID string) error {
	client, normalizedID, err := c.managedUserClient(authUserID)
	if err != nil {
		return err
	}
	response, err := client.RevokeUserSessionsWithResponse(ctx, internalcontract.RevokeUserSessionsJSONRequestBody{UserID: normalizedID})
	if err != nil {
		return err
	}
	if response.JSON200 == nil {
		_, err := adminMutationResult("revoke user sessions", response.StatusCode(), response.Body, 0, "", false)
		return err
	}
	dataFields, err := adminMutationResult(
		"revoke user sessions", response.StatusCode(), response.Body,
		int(response.JSON200.Code), response.JSON200.Msg, true,
	)
	if err != nil {
		return err
	}
	data := response.JSON200.Data
	return validateMutationSuccess(
		"revoke user sessions", normalizedID, data.AuthUserID,
		dataFields, "revoked", bool(data.Revoked),
	)
}

func (c *ServiceClient) contractClient() (*internalcontract.ClientWithResponses, error) {
	if c == nil {
		return nil, fmt.Errorf("auth service client not configured")
	}
	if c.initErr != nil {
		return nil, fmt.Errorf("configure auth service client: %w", c.initErr)
	}
	if c.contract == nil {
		return nil, fmt.Errorf("auth service client not configured")
	}
	return c.contract, nil
}

func (c *ServiceClient) managedUserClient(authUserID string) (*internalcontract.ClientWithResponses, string, error) {
	client, err := c.contractClient()
	if err != nil {
		return nil, "", err
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return nil, "", fmt.Errorf("auth user id is required")
	}
	return client, normalizedID, nil
}

func withCookieHeader(cookieHeader string) internalcontract.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if header := strings.TrimSpace(cookieHeader); header != "" {
			req.Header.Set("Cookie", header)
		}
		return nil
	}
}

func adminMutationResult(action string, status int, body []byte, code int, message string, hasSuccess bool) (map[string]json.RawMessage, error) {
	if status != http.StatusOK {
		failure, err := decodeContractFailure(body)
		if err != nil {
			return nil, fmt.Errorf("auth service %s failed with status %d: %w", action, status, err)
		}
		if err := validateContractFailureStatus(status, failure); err != nil {
			return nil, fmt.Errorf("auth service %s returned an invalid error envelope: %w", action, err)
		}
		if failure.Code == http.StatusNotFound {
			return nil, ErrAuthUserNotFound
		}
		return nil, contractFailureWithEnvelope(action, status, failure)
	}
	if !hasSuccess {
		return nil, invalidMutationSuccess(action)
	}
	fields, err := requireSuccessEnvelopeFields(body)
	if err != nil {
		return nil, fmt.Errorf("auth service %s returned an invalid success envelope: %w", action, err)
	}
	if code != 0 {
		return nil, fmt.Errorf("%w: code %d: %s", invalidMutationSuccess(action), code, nonEmpty(message, "unknown error"))
	}
	dataJSON, err := requireJSONField(fields, "data")
	if err != nil {
		return nil, fmt.Errorf("auth service %s returned an invalid success envelope: %w", action, err)
	}
	dataFields, err := decodeJSONFields(dataJSON)
	if err != nil {
		return nil, fmt.Errorf("auth service %s returned an invalid success envelope: data: %w", action, err)
	}
	return dataFields, nil
}

func contractFailure(action string, status int, body []byte) error {
	failure, err := decodeContractFailure(body)
	if err != nil {
		return fmt.Errorf("auth service %s failed with status %d: %w", action, status, err)
	}
	if err := validateContractFailureStatus(status, failure); err != nil {
		return fmt.Errorf("auth service %s returned an invalid error envelope: %w", action, err)
	}
	return contractFailureWithEnvelope(action, status, failure)
}

func validateContractFailureStatus(status int, failure internalcontract.ErrorEnvelope) error {
	if failure.Code != status {
		return fmt.Errorf("error code %d does not match HTTP status %d", failure.Code, status)
	}
	return nil
}

func contractFailureWithEnvelope(action string, status int, failure internalcontract.ErrorEnvelope) error {
	if failure.Msg != "" {
		return fmt.Errorf("auth service %s failed: %s", action, failure.Msg)
	}
	return fmt.Errorf("auth service %s failed with status %d", action, status)
}

func decodeContractFailure(body []byte) (internalcontract.ErrorEnvelope, error) {
	fields, err := decodeJSONFields(body)
	if err != nil {
		return internalcontract.ErrorEnvelope{}, fmt.Errorf("decode error envelope: %w", err)
	}
	codeJSON, err := requireJSONField(fields, "code")
	if err != nil {
		return internalcontract.ErrorEnvelope{}, fmt.Errorf("decode error envelope: %w", err)
	}
	messageJSON, err := requireJSONField(fields, "msg")
	if err != nil {
		return internalcontract.ErrorEnvelope{}, fmt.Errorf("decode error envelope: %w", err)
	}

	var code *int
	if err := json.Unmarshal(codeJSON, &code); err != nil || code == nil {
		if err == nil {
			err = fmt.Errorf("expected integer")
		}
		return internalcontract.ErrorEnvelope{}, fmt.Errorf("decode error envelope: code: %w", err)
	}
	var message *string
	if err := json.Unmarshal(messageJSON, &message); err != nil || message == nil {
		if err == nil {
			err = fmt.Errorf("expected string")
		}
		return internalcontract.ErrorEnvelope{}, fmt.Errorf("decode error envelope: msg: %w", err)
	}
	return internalcontract.ErrorEnvelope{Code: *code, Msg: *message}, nil
}

func decodeJSONFields(body []byte) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	return fields, nil
}

func requireSuccessEnvelopeFields(body []byte) (map[string]json.RawMessage, error) {
	fields, err := decodeJSONFields(body)
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"code", "msg", "data"} {
		if _, err := requireJSONField(fields, name); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func requireJSONField(fields map[string]json.RawMessage, name string) (json.RawMessage, error) {
	value, ok := fields[name]
	if !ok {
		return nil, fmt.Errorf("missing %q field", name)
	}
	return value, nil
}

func invalidMutationSuccess(action string) error {
	return fmt.Errorf("auth service %s returned an invalid success envelope", action)
}

func validateMutationSuccess(
	action string,
	expectedAuthUserID string,
	actualAuthUserID string,
	dataFields map[string]json.RawMessage,
	resultField string,
	resultMatches bool,
) error {
	if _, err := requireJSONField(dataFields, resultField); err != nil ||
		actualAuthUserID != expectedAuthUserID || !resultMatches {
		return invalidMutationSuccess(action)
	}
	return nil
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeEnsureRole(role ...string) internalcontract.AuthRole {
	if len(role) == 0 {
		return internalcontract.User
	}
	value := internalcontract.AuthRole(strings.ToLower(strings.TrimSpace(role[0])))
	if value.Valid() {
		return value
	}
	return internalcontract.User
}

type boundedResponseTransport struct {
	base     http.RoundTripper
	maxBytes int64
}

func (t *boundedResponseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.Body != nil {
		response.Body = &boundedResponseBody{
			ReadCloser: response.Body,
			remaining:  t.maxBytes,
		}
	}
	return response, nil
}

type boundedResponseBody struct {
	io.ReadCloser
	remaining int64
}

func (b *boundedResponseBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if b.remaining == 0 {
		var probe [1]byte
		read, err := b.ReadCloser.Read(probe[:])
		if read > 0 {
			return 0, errAuthServiceResponseTooLarge
		}
		return 0, err
	}
	if int64(len(buffer)) > b.remaining {
		buffer = buffer[:b.remaining]
	}
	read, err := b.ReadCloser.Read(buffer)
	b.remaining -= int64(read)
	return read, err
}
