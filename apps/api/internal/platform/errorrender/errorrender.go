// Package errorrender is the single error-handling home: taxonomy,
// validation sentinel, and HTTP rendering converge here.
//
// Each domain keeps its row table with wire-stable code/copy. Matching uses
// errors.Is with first-win order; rendering goes through response.MachineError
// (or the legacy numeric envelope for payments defaults) so the safe-message
// trust boundary holds in one seam. Default/unmatched failures log at Error
// level with domain-supplied messages and attrs; 4xx matches render without
// logging.
//
// The former platform/apperror (taxonomy.go) and platform/validation
// (validation.go) implementations moved here; those packages remain as
// deprecated aliases over this implementation — no duplicated logic.
package errorrender

import (
	"errors"
	"log/slog"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// Matcher is any per-domain row able to report errors.Is membership.
// Domains keep their own row structs for wire-stable tables; the first-win
// loop lives here so the domains share one matching core.
type Matcher interface {
	Matches(error) bool
}

// TargetsMatch reports whether err matches any target via errors.Is.
func TargetsMatch(targets []error, err error) bool {
	for _, target := range targets {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// SingleMatch reports errors.Is membership for single-target rows (boards).
func SingleMatch(target, err error) bool {
	return errors.Is(err, target)
}

// FirstMatch returns the first table row whose Matches reports true.
// Rows are ordered most-specific first; the first match wins.
func FirstMatch[T Matcher](table []T, err error) (T, bool) {
	for _, row := range table {
		if row.Matches(err) {
			return row, true
		}
	}
	var zero T
	return zero, false
}

// Render renders a matched row through the machine envelope. Empty-message
// fallback and match logging stay in the owning package so redacted domain
// summaries keep their existing shape; this seam only enforces the
// MachineError trust boundary.
func Render(c *gin.Context, status int, code, message string) {
	response.MachineError(c, status, code, message)
}

// RenderDefault logs an unmatched/default failure at Error level and renders
// the machine envelope. Extra attrs (e.g. error_type) stay domain-supplied.
// A nil err omits the err attr so misconfiguration paths without a cause
// keep their existing log shape.
func RenderDefault(c *gin.Context, logMsg string, status int, code, message string, err error, attrs ...any) {
	args := make([]any, 0, len(attrs)+12)
	args = append(args, "status", status, "code", code)
	args = append(args, attrs...)
	if c != nil {
		args = append(args, "request_id", c.GetString(response.RequestIDHeader))
		if c.Request != nil {
			args = append(args, "method", c.Request.Method, "path", c.Request.URL.Path)
		}
	}
	if err != nil {
		args = append(args, "err", err)
	}
	slog.Error(logMsg, args...)
	response.MachineError(c, status, code, message)
}

// RenderNumeric renders the legacy numeric envelope for payments defaults.
// Those defaults predate the machine-code contract and carry no stable code;
// the envelope stays numeric so the wire does not change.
func RenderNumeric(c *gin.Context, status int, message string) {
	response.Error(c, status, message)
}
