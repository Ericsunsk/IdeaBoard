package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ideaboard/api/internal/identity/auth/internalcontract"
	"ideaboard/api/internal/identity/autherrors"
	"ideaboard/api/internal/platform/config"
)

var (
	ErrSessionUnauthorized         = errors.New("auth session unauthorized")
	ErrAuthUserNotFound            = autherrors.ErrAuthUserNotFound
	errAuthServiceResponseTooLarge = errors.New("auth service response body exceeds 2 MiB")
)

const maxAuthServiceResponseBytes int64 = 2 * 1024 * 1024

type SessionIdentity struct {
	AuthUserID            string `json:"auth_user_id"`
	Username              string `json:"username"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
}

type ProvisionedAuthUser struct {
	AuthUserID            string `json:"auth_user_id"`
	Username              string `json:"username"`
	ClaimIdentity         string `json:"claim_identity"`
	ClaimIdentityKind     string `json:"claim_identity_kind"`
	ClaimIdentityVerified bool   `json:"claim_identity_verified"`
	Role                  string `json:"role"`
	Existing              bool   `json:"existing"`
}

type ServiceClient struct {
	contract   *internalcontract.ClientWithResponses
	httpClient *http.Client
	initErr    error
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
	return &ServiceClient{contract: client, httpClient: httpClient, initErr: err}
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
	session := SessionIdentity{
		AuthUserID:            contractSession.AuthUserID,
		Username:              contractSession.Username,
		ClaimIdentity:         contractSession.ClaimIdentity,
		ClaimIdentityKind:     string(contractSession.ClaimIdentityKind),
		ClaimIdentityVerified: contractSession.ClaimIdentityVerified,
	}
	if strings.TrimSpace(session.AuthUserID) == "" {
		return nil, fmt.Errorf("auth service returned empty auth_user_id")
	}
	if strings.TrimSpace(session.ClaimIdentity) == "" ||
		session.ClaimIdentityKind != string(internalcontract.SessionClaimIdentityKindTrustedCanonicalRegistration) ||
		!session.ClaimIdentityVerified {
		return nil, fmt.Errorf("%w: auth service returned an untrusted session identity", ErrSessionUnauthorized)
	}
	return &session, nil
}

func (c *ServiceClient) EnsureUser(ctx context.Context, username, password string, allowExisting bool, role ...string) (*ProvisionedAuthUser, error) {
	client, err := c.contractClient()
	if err != nil {
		return nil, err
	}
	response, err := client.EnsureUserWithResponse(ctx, internalcontract.EnsureUserJSONRequestBody{
		AllowExisting: allowExisting,
		Password:      password,
		Role:          normalizeEnsureRole(role...),
		Username:      strings.TrimSpace(username),
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
		Username:              contractUser.Username,
		ClaimIdentity:         contractUser.ClaimIdentity,
		ClaimIdentityKind:     string(contractUser.ClaimIdentityKind),
		ClaimIdentityVerified: bool(contractUser.ClaimIdentityVerified),
		Role:                  string(contractUser.Role),
		Existing:              contractUser.Existing,
	}
	if strings.TrimSpace(user.AuthUserID) == "" {
		return nil, fmt.Errorf("auth service returned empty auth_user_id")
	}
	if strings.TrimSpace(user.ClaimIdentity) == "" ||
		user.ClaimIdentityKind != string(internalcontract.TrustedClaimIdentityKindTrustedCanonicalRegistration) ||
		!user.ClaimIdentityVerified {
		return nil, fmt.Errorf("auth service returned an untrusted claim identity")
	}
	return &user, nil
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
