package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"api/internal/platform/config"
)

var (
	ErrSessionUnauthorized = errors.New("auth session unauthorized")
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
	return &ServiceClient{
		baseURL:        strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/"),
		internalAPIKey: strings.TrimSpace(cfg.InternalAPIKey),
		httpClient: &http.Client{
			Timeout: timeout,
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
