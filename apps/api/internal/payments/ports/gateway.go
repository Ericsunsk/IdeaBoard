package ports

import "strings"

type GatewayErrorCode string

const (
	GatewayErrorInvalidKey          GatewayErrorCode = "invalid_key"
	GatewayErrorPermissionDenied    GatewayErrorCode = "permission_denied"
	GatewayErrorAccountUnavailable  GatewayErrorCode = "account_unavailable"
	GatewayErrorCapabilityMissing   GatewayErrorCode = "capability_missing"
	GatewayErrorProviderUnavailable GatewayErrorCode = "provider_unavailable"
	GatewayErrorInvalidResponse     GatewayErrorCode = "invalid_response"
	GatewayErrorRequestRejected     GatewayErrorCode = "request_rejected"
)

type GatewayError struct {
	code        GatewayErrorCode
	safeMessage string
	cause       error
}

func NewGatewayError(code GatewayErrorCode, safeMessage string, cause error) *GatewayError {
	return &GatewayError{code: code, safeMessage: strings.TrimSpace(safeMessage), cause: cause}
}

func (e *GatewayError) Error() string {
	if e == nil || e.safeMessage == "" {
		return "Stripe operation failed"
	}
	return e.safeMessage
}

func (e *GatewayError) Code() GatewayErrorCode {
	if e == nil {
		return GatewayErrorProviderUnavailable
	}
	return e.code
}
