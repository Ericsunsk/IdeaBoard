package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ideaboard/api/internal/autherrors"
	"ideaboard/api/internal/platform/config"
)

var (
	ErrSessionUnauthorized = errors.New("auth session unauthorized")
	ErrAuthUserNotFound    = autherrors.ErrAuthUserNotFound
)

type SessionIdentity struct {
	AuthUserID string `json:"auth_user_id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
}

type ProvisionedAuthUser struct {
	AuthUserID string `json:"auth_user_id"`
	Username   string `json:"username"`
	Email      string `json:"email"`
	Role       string `json:"role"`
	Existing   bool   `json:"existing"`
}

type ServiceClient struct {
	baseURL        string
	internalAPIKey string
	httpClient     *http.Client
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

	return &ServiceClient{
		baseURL:        strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		internalAPIKey: strings.TrimSpace(cfg.InternalAPIKey),
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

func (c *ServiceClient) ValidateSession(ctx context.Context, cookieHeader string) (*SessionIdentity, error) {
	if c == nil {
		return nil, fmt.Errorf("auth service client not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/internal/session", nil)
	if err != nil {
		return nil, err
	}
	if header := strings.TrimSpace(cookieHeader); header != "" {
		req.Header.Set("Cookie", header)
	}

	var resp authServiceEnvelope
	status, err := c.doJSON(req, &resp)
	if err != nil {
		return nil, err
	}
	if status == http.StatusUnauthorized {
		return nil, ErrSessionUnauthorized
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("auth service session check failed with status %d", status)
	}
	if resp.Code != 0 {
		if resp.Code == http.StatusUnauthorized {
			return nil, ErrSessionUnauthorized
		}
		return nil, fmt.Errorf("auth service session rejected: %s", nonEmpty(resp.Msg, "unknown error"))
	}
	session := resp.Data.Session
	if strings.TrimSpace(session.AuthUserID) == "" {
		return nil, fmt.Errorf("auth service returned empty auth_user_id")
	}
	return &session, nil
}

func (c *ServiceClient) EnsureUser(ctx context.Context, username, password string, allowExisting bool, role ...string) (*ProvisionedAuthUser, error) {
	if c == nil {
		return nil, fmt.Errorf("auth service client not configured")
	}
	targetRole := normalizeEnsureRole(role...)
	payload := map[string]any{
		"username":       strings.TrimSpace(username),
		"password":       password,
		"allow_existing": allowExisting,
		"role":           targetRole,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/users/ensure", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	var resp authServiceEnvelope
	status, err := c.doJSON(req, &resp)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		if resp.Msg != "" {
			return nil, fmt.Errorf("auth service ensure user failed: %s", resp.Msg)
		}
		return nil, fmt.Errorf("auth service ensure user failed with status %d", status)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("auth service ensure user rejected: %s", nonEmpty(resp.Msg, "unknown error"))
	}
	user := resp.Data.User
	if strings.TrimSpace(user.AuthUserID) == "" {
		return nil, fmt.Errorf("auth service returned empty auth_user_id")
	}
	return &user, nil
}

func (c *ServiceClient) DeleteUser(ctx context.Context, authUserID string) error {
	if c == nil {
		return fmt.Errorf("auth service client not configured")
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return fmt.Errorf("auth user id is required")
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		c.baseURL+"/internal/users/"+url.PathEscape(normalizedID),
		nil,
	)
	if err != nil {
		return err
	}

	var resp authServiceEnvelope
	status, err := c.doJSON(req, &resp)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || resp.Code == http.StatusNotFound {
		return ErrAuthUserNotFound
	}
	if status != http.StatusOK {
		if resp.Msg != "" {
			return fmt.Errorf("auth service delete user failed: %s", resp.Msg)
		}
		return fmt.Errorf("auth service delete user failed with status %d", status)
	}
	if resp.Code != 0 {
		return fmt.Errorf("auth service delete user rejected: %s", nonEmpty(resp.Msg, "unknown error"))
	}
	return nil
}

func (c *ServiceClient) SetUserRole(ctx context.Context, authUserID string, role string) error {
	if c == nil {
		return fmt.Errorf("auth service client not configured")
	}
	normalizedID := strings.TrimSpace(authUserID)
	normalizedRole := normalizeEnsureRole(role)
	if normalizedID == "" {
		return fmt.Errorf("auth user id is required")
	}

	payload := map[string]any{
		"role": normalizedRole,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPut,
		c.baseURL+"/internal/users/"+url.PathEscape(normalizedID)+"/role",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	var resp authServiceEnvelope
	status, err := c.doJSON(req, &resp)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || resp.Code == http.StatusNotFound {
		return ErrAuthUserNotFound
	}
	if status != http.StatusOK {
		if resp.Msg != "" {
			return fmt.Errorf("auth service set user role failed: %s", resp.Msg)
		}
		return fmt.Errorf("auth service set user role failed with status %d", status)
	}
	if resp.Code != 0 {
		return fmt.Errorf("auth service set user role rejected: %s", nonEmpty(resp.Msg, "unknown error"))
	}
	return nil
}

func (c *ServiceClient) BanUser(ctx context.Context, authUserID string, reason string, expiresInSeconds int) error {
	if c == nil {
		return fmt.Errorf("auth service client not configured")
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return fmt.Errorf("auth user id is required")
	}
	payload := map[string]any{
		"userId": normalizedID,
	}
	if reason := strings.TrimSpace(reason); reason != "" {
		payload["banReason"] = reason
	}
	if expiresInSeconds > 0 {
		payload["banExpiresIn"] = expiresInSeconds
	}
	return c.callInternalAdminMutation(ctx, "/internal/users/ban", payload, "ban")
}

func (c *ServiceClient) UnbanUser(ctx context.Context, authUserID string) error {
	if c == nil {
		return fmt.Errorf("auth service client not configured")
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return fmt.Errorf("auth user id is required")
	}
	return c.callInternalAdminMutation(ctx, "/internal/users/unban", map[string]any{
		"userId": normalizedID,
	}, "unban")
}

func (c *ServiceClient) RevokeUserSessions(ctx context.Context, authUserID string) error {
	if c == nil {
		return fmt.Errorf("auth service client not configured")
	}
	normalizedID := strings.TrimSpace(authUserID)
	if normalizedID == "" {
		return fmt.Errorf("auth user id is required")
	}
	return c.callInternalAdminMutation(ctx, "/internal/users/revoke-sessions", map[string]any{
		"userId": normalizedID,
	}, "revoke user sessions")
}

func (c *ServiceClient) callInternalAdminMutation(ctx context.Context, path string, payload map[string]any, action string) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	var resp authServiceEnvelope
	status, err := c.doJSON(req, &resp)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || resp.Code == http.StatusNotFound {
		return ErrAuthUserNotFound
	}
	if status != http.StatusOK {
		if resp.Msg != "" {
			return fmt.Errorf("auth service %s failed: %s", action, resp.Msg)
		}
		return fmt.Errorf("auth service %s failed with status %d", action, status)
	}
	if resp.Code != 0 {
		return fmt.Errorf("auth service %s rejected: %s", action, nonEmpty(resp.Msg, "unknown error"))
	}
	return nil
}

func (c *ServiceClient) doJSON(req *http.Request, out *authServiceEnvelope) (int, error) {
	if c.internalAPIKey != "" {
		req.Header.Set("X-Internal-Key", c.internalAPIKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return resp.StatusCode, err
	}
	if len(raw) == 0 {
		if out != nil {
			*out = authServiceEnvelope{}
		}
		return resp.StatusCode, nil
	}
	if out == nil {
		return resp.StatusCode, nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return resp.StatusCode, fmt.Errorf("auth service invalid json response: %w", err)
	}
	return resp.StatusCode, nil
}

type authServiceEnvelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Session SessionIdentity     `json:"session"`
		User    ProvisionedAuthUser `json:"user"`
	} `json:"data"`
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func normalizeEnsureRole(role ...string) string {
	if len(role) == 0 {
		return "user"
	}
	value := strings.ToLower(strings.TrimSpace(role[0]))
	if value == "admin" || value == "user" {
		return value
	}
	return "user"
}
