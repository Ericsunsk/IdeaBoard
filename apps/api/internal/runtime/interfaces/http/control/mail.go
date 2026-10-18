package control

// Control transactional mail configuration delegates to the Auth-owned
// ControlMailService. The guard supplies authorization, rate limits,
// header-only idempotency, and audit; Auth owns secrets and provider lifecycle.

import (
	"context"
	"errors"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"net/http"
	"strings"

	"ideaboard/api/internal/identity/auth"
	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// ControlMailService is the Auth-owned transactional mail seam behind the
// control mail-setting handlers. *auth.ServiceClient satisfies it;
// tests substitute a fake.
type ControlMailService interface {
	GetMailConfig(ctx context.Context) (*auth.MailConfigSummary, error)
	UpdateMailConfig(ctx context.Context, input auth.MailConfigUpdate, actorID int64) (*auth.MailConfigSummary, error)
	ClearMailConfig(ctx context.Context, actorID int64) error
	SendMailConfigTest(ctx context.Context, recipient string, actorID int64) error
}

// controlMailAck is the contract-envelope acknowledgement for mail
// mutations without a resource body (clear/test). The data object contains
// deleted or sent flags.
type controlMailAck struct {
	Deleted bool `json:"deleted,omitempty"`
	Sent    bool `json:"sent,omitempty"`
}

func (d *ContractControlDeps) requireControlMail(c *gin.Context) (ControlMailService, bool) {
	if d == nil || d.Mail == nil {
		contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable10)
		return nil, false
	}
	return d.Mail, true
}

func (d *ContractControlDeps) getControlMailSetting(c *gin.Context, _ controlContext) {
	mail, ok := d.requireControlMail(c)
	if !ok {
		return
	}
	summary, err := mail.GetMailConfig(c.Request.Context())
	if err != nil {
		writeControlMailError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}

func (d *ContractControlDeps) updateControlMailSetting(c *gin.Context, ctx controlContext) {
	mail, ok := d.requireControlMail(c)
	if !ok {
		return
	}
	var req struct {
		APIKey    string `json:"api_key"`
		FromName  string `json:"from_name"`
		FromEmail string `json:"from_email"`
		ReplyTo   string `json:"reply_to"`
		Enabled   *bool  `json:"enabled"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	summary, err := mail.UpdateMailConfig(c.Request.Context(), auth.MailConfigUpdate{
		APIKey: req.APIKey, FromName: req.FromName, FromEmail: req.FromEmail, ReplyTo: req.ReplyTo,
		Enabled: req.Enabled,
	}, ctx.Operator.UserID())
	if err != nil {
		writeControlMailError(c, err)
		return
	}
	contract.RenderSuccess(c, summary)
}

func (d *ContractControlDeps) deleteControlMailSetting(c *gin.Context, ctx controlContext) {
	mail, ok := d.requireControlMail(c)
	if !ok {
		return
	}
	if err := mail.ClearMailConfig(c.Request.Context(), ctx.Operator.UserID()); err != nil {
		writeControlMailError(c, err)
		return
	}
	contract.RenderSuccess(c, controlMailAck{Deleted: true})
}

func (d *ContractControlDeps) testControlMailSetting(c *gin.Context, ctx controlContext) {
	mail, ok := d.requireControlMail(c)
	if !ok {
		return
	}
	// This detached mutation requires an Idempotency-Key.
	var req struct {
		Recipient string `json:"recipient"`
	}
	if err := contract.BindStrictJSON(c, &req); err != nil {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest4)
		return
	}
	recipient := strings.TrimSpace(req.Recipient)
	if recipient == "" {
		contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest, runtimehttp.MsgBadRequest19)
		return
	}
	// External probe: the Auth send runs outside any guard-owned DB
	// transaction (detached mutation). Only a nil error renders
	// {sent:true}; delivery failures and unknown results map to
	// non-2xx (never success) and are never cached by the idempotency
	// seam, so a retry with the same key re-sends the probe. A
	// transaction rollback elsewhere never implies the sent probe was
	// revoked.
	if err := mail.SendMailConfigTest(c.Request.Context(), recipient, ctx.Operator.UserID()); err != nil {
		writeControlMailError(c, err)
		return
	}
	contract.RenderSuccess(c, controlMailAck{Sent: true})
}

// writeControlMailError maps the Auth mail seam failures onto the control
// envelope. Auth validation (400) and missing-configuration (409) keep
// their status; delivery failures and unknown results (Auth 502/500,
// transport errors, invalid envelopes) fail closed as auth unavailable
// (503) without leaking provider details and never as success. The
// idempotency seam only caches 2xx, so these failures re-execute on
// retry instead of replaying.
func writeControlMailError(c *gin.Context, err error) {
	var configErr *auth.MailConfigError
	if errors.As(err, &configErr) {
		switch configErr.Status {
		case http.StatusBadRequest:
			contract.RenderError(c, http.StatusBadRequest, contract.CodeBadRequest,
				nonEmptyControlMessage(configErr.Message, runtimehttp.MsgBadRequest4))
			return
		case http.StatusConflict:
			contract.RenderError(c, http.StatusConflict, contract.CodeConflict,
				nonEmptyControlMessage(configErr.Message, runtimehttp.MsgWriteControlMailError))
			return
		}
	}
	contract.RenderError(c, http.StatusServiceUnavailable, contract.CodeServiceUnavailable, runtimehttp.MsgServiceUnavailable11)
}
