// Shared table-driven HTTP error mapping (P0-6 convergence).
//
// Every edge package declares its wire table as []Mapping and renders
// through RenderMapped: the matching core (errors.Is, first-win order,
// scope filtering), the machine-envelope trust boundary, and the logging
// policy live here exactly once. Domains keep only data (rows with
// wire-stable code/copy) plus a thin writer that passes its frozen
// default and logging knobs.
//
// Rules:
//   - Rows are ordered most-specific first; the first in-scope match wins.
//   - Scope "" participates in every scope; a non-empty render scope only
//     selects rows with Scope "" or the same Scope (rewards admin/redeem).
//   - Message constants stay byte-identical at the owning edge package;
//     this seam never invents copy. A matched row with an empty Message is
//     a programming error: it logs and falls back to FallbackMessage (or
//     the default message when unset) so the wire never leaks raw errors.
//   - Unmatched errors log only when UnmatchedLog is set (rewards/billing/
//     credits); silent edges (payments numeric default, vision) leave it
//     empty. Attrs callbacks keep domain-specific attribution (redacted
//     summaries, error_type) without spreading slog call sites.
package errorrender

import (
	"fmt"
	"log/slog"

	"ideaboard/api/internal/platform/response"

	"github.com/gin-gonic/gin"
)

// Mapping binds one stable outcome to its wire rendering. Code is the
// wire-stable machine code from platform/contract; Message is the frozen
// safe copy owned by the edge package; Scope partitions one table across
// surfaces ("" = all scopes).
type Mapping struct {
	Targets []error
	Status  int
	Code    string
	Message string
	Scope   string
}

// Matches reports whether err matches any target via errors.Is.
func (m Mapping) Matches(err error) bool {
	return TargetsMatch(m.Targets, err)
}

// InScope reports whether the row participates in the render scope.
// Empty row scope matches every scope; empty render scope matches all rows.
func (m Mapping) InScope(scope string) bool {
	return m.Scope == "" || scope == "" || m.Scope == scope
}

// MappedOptions configures one RenderMapped call. DefaultStatus/Code/
// Message are the frozen unmatched outcome; the remaining knobs preserve
// each edge's logging contract.
type MappedOptions struct {
	// Scope selects table rows (see InScope). Empty matches all rows.
	Scope string
	// DefaultStatus, DefaultCode, DefaultMessage render unmatched errors.
	DefaultStatus  int
	DefaultCode    string
	DefaultMessage string
	// FallbackMessage renders matched rows with an empty Message
	// (programmer-error guard). Empty falls back to DefaultMessage.
	FallbackMessage string
	// NumericDefault renders the unmatched default through the legacy
	// numeric envelope (pre-machine-code edges). Matched rows always use
	// the machine envelope.
	NumericDefault bool
	// UnmatchedLog enables Error-level logging for unmatched errors.
	// Empty disables it (silent defaults).
	UnmatchedLog string
	// UnmatchedAttrs derives extra slog attrs from err. Nil logs the raw
	// err attr; rewards passes redaction here and sets OmitUnmatchedErr.
	UnmatchedAttrs func(error) []any
	// OmitUnmatchedErr omits the raw err attr (edges whose errors carry
	// sensitive material log a redacted summary instead).
	OmitUnmatchedErr bool
	// MatchedLogAtOrAbove logs matched rows with Status >= the threshold
	// (5xx attribution for billing/credits). 0 disables it.
	MatchedLogAtOrAbove int
	// MatchedLog is the slog message for matched-row logging.
	MatchedLog string
	// MatchedAttrs derives extra slog attrs from the matched row + err.
	MatchedAttrs func(Mapping, error) []any
}

// RenderMapped renders err through table: the first in-scope match wins
// via the machine envelope; unmatched errors render the default (logged
// only when UnmatchedLog is set).
func RenderMapped(c *gin.Context, err error, table []Mapping, opts MappedOptions) {
	for _, row := range table {
		if !row.InScope(opts.Scope) {
			continue
		}
		if !row.Matches(err) {
			continue
		}
		if opts.MatchedLogAtOrAbove != 0 && row.Status >= opts.MatchedLogAtOrAbove {
			args := []any{"status", row.Status, "code", row.Code}
			if opts.MatchedAttrs != nil {
				args = append(args, opts.MatchedAttrs(row, err)...)
			} else {
				args = append(args, "err", err)
			}
			args = appendRequestAttrs(c, args)
			slog.Error(coalesceLogMessage(opts.MatchedLog, "mapped error"), args...)
		}
		message := row.Message
		if message == "" {
			slog.Error("error mapping missing safe message",
				"code", row.Code, "status", row.Status, "err", err)
			message = opts.FallbackMessage
			if message == "" {
				message = opts.DefaultMessage
			}
		}
		Render(c, row.Status, row.Code, message)
		return
	}
	if opts.UnmatchedLog != "" {
		args := []any{"status", opts.DefaultStatus, "code", opts.DefaultCode}
		if opts.UnmatchedAttrs != nil {
			args = append(args, opts.UnmatchedAttrs(err)...)
		}
		if !opts.OmitUnmatchedErr {
			args = append(args, "err", err)
		}
		args = appendRequestAttrs(c, args)
		slog.Error(opts.UnmatchedLog, args...)
	}
	if opts.NumericDefault {
		RenderNumeric(c, opts.DefaultStatus, opts.DefaultMessage)
		return
	}
	Render(c, opts.DefaultStatus, opts.DefaultCode, opts.DefaultMessage)
}

// MatchErrorTypeAttrWithErr derives error_type plus the raw err attr for
// edges whose failures carry no sensitive material (billing 5xx).
func MatchErrorTypeAttrWithErr(_ Mapping, err error) []any {
	return []any{"error_type", fmt.Sprintf("%T", err), "err", err}
}

func appendRequestAttrs(c *gin.Context, args []any) []any {
	if c != nil {
		if id := c.GetString(response.RequestIDHeader); id != "" {
			args = append(args, "request_id", id)
		}
		if c.Request != nil {
			args = append(args, "method", c.Request.Method, "path", c.Request.URL.Path)
		}
	}
	return args
}

// UnmatchedErrorTypeAttr derives the shared error_type attr for
// unmatched logging.
func UnmatchedErrorTypeAttr(err error) []any {
	return []any{"error_type", fmt.Sprintf("%T", err)}
}

func coalesceLogMessage(message, fallback string) string {
	if message != "" {
		return message
	}
	return fallback
}
