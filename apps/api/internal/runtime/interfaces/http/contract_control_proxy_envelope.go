package httpapi

// Control Plane proxy envelope unification (parent #809, Baseline §12).
//
// Every controlProxy route reuses a legacy admin handler method so
// validation and side effects stay single-sourced in the owning domain.
// The legacy methods render the legacy admin shapes directly
// (platform/response numeric {code,msg,data} successes and either numeric
// {code,msg} or machine {code,msg} errors). translateControlProxyResponse
// captures that output at the control boundary and re-renders it in the
// platform/contract envelope ({data} success, {error:{code,message,
// request_id}} errors) with the Request-Id correlation preserved from the
// same gin context.
//
// Shape only: HTTP statuses are replayed unchanged, domain handlers are
// untouched, and error messages keep their safe legacy copy. Error codes
// map to the stable per-family contract codes: machine-code families
// (rewards, storage, search admin, statistics) keep their wire-stable
// string codes verbatim; numeric-legacy families (ai-gateway, feature
// prompts, brand-logo overrides, monitor) map by status to the kernel
// generic codes.
//
// Protocol exceptions bypass translation via controlProxyRaw: the
// ai-gateway brand-logo (binary image/svg+xml) and the monitor SSE stream
// (text/event-stream) are never JSON envelopes.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// controlProxyCaptureWriter buffers a proxied legacy handler response so
// the boundary can translate it. Headers land in an isolated map: only
// the translated contract body is replayed, never legacy transport
// headers. The status defaults to 200 on first Write, mirroring net/http.
type controlProxyCaptureWriter struct {
	gin.ResponseWriter
	header      http.Header
	status      int
	wroteHeader bool
	body        bytes.Buffer
}

func (w *controlProxyCaptureWriter) Header() http.Header {
	return w.header
}

func (w *controlProxyCaptureWriter) WriteHeader(status int) {
	if w == nil || w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
}

func (w *controlProxyCaptureWriter) Write(data []byte) (int, error) {
	if w == nil {
		return 0, nil
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(data)
}

// controlProxyRaw adapts a legacy admin handler method to a guarded
// control route WITHOUT envelope translation, for the two protocol
// exceptions (binary brand-logo, SSE monitor stream) that are never JSON
// envelopes. available reports whether the handler is wired; nil stays
// fail-closed with 501 instead of panicking.
func (d *ContractControlDeps) controlProxyRaw(route controlRoute, capability string, available bool, method func(*gin.Context)) gin.HandlerFunc {
	return d.guard(capability, route, func(c *gin.Context, _ controlContext) {
		if !available {
			contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, msgContractNotImplemented)
			return
		}
		method(c)
	})
}

// translateControlProxyResponse runs a legacy admin handler with a
// capturing writer and re-renders the captured output in the contract
// envelope on the real writer, preserving the captured HTTP status.
// Non-JSON payloads replay byte-identical (defensive; the known binary
// and SSE routes use controlProxyRaw and never reach translation).
func translateControlProxyResponse(c *gin.Context, method func(*gin.Context)) {
	capture := &controlProxyCaptureWriter{ResponseWriter: c.Writer, header: make(http.Header)}
	c.Writer = capture
	method(c)
	c.Writer = capture.ResponseWriter

	status := capture.status
	raw := capture.body.Bytes()
	if !capture.wroteHeader && len(raw) == 0 {
		contract.RenderError(c, http.StatusInternalServerError, contract.CodeServiceUnavailable, msgServiceUnavailable24)
		return
	}
	if status == 0 {
		status = http.StatusOK
	}
	if !isControlProxyJSON(capture.header.Get("Content-Type"), raw) {
		for key, values := range capture.header {
			for _, value := range values {
				c.Header(key, value)
			}
		}
		// Non-JSON protocol payloads replay byte-identical (defensive;
		// the known binary and SSE routes use controlProxyRaw and never
		// reach translation).
		c.Data(status, capture.header.Get("Content-Type"), raw)
		return
	}
	if status >= 400 {
		renderControlProxyError(c, status, raw)
		return
	}
	renderControlProxySuccess(c, status, raw)
}

// isControlProxyJSON reports whether the captured payload is a JSON
// envelope candidate. Legacy handlers render application/json; an empty
// body or a JSON sniff match also translates (as an empty-ack object).
func isControlProxyJSON(contentType string, raw []byte) bool {
	if trimmed := strings.TrimSpace(contentType); trimmed != "" {
		media := strings.ToLower(strings.TrimSpace(strings.Split(trimmed, ";")[0]))
		return media == "" || media == "application/json" || strings.HasSuffix(media, "+json")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return true
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

// renderControlProxySuccess unwraps the legacy numeric envelope's data
// field into {data}. A captured success without a data member (the
// legacy nil-ack shape {code:0,msg:success}) renders {data:{}} so the
// acknowledgement stays an object per ControlAckEnvelope. Rendering goes
// through the contract Render* seam (never raw c.JSON) so the
// TestErrorResponsesStayInsideSharedEnvelope trust boundary holds.
func renderControlProxySuccess(c *gin.Context, status int, raw []byte) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		renderControlProxyData(c, status, map[string]any{})
		return
	}
	var decoded any
	if err := json.Unmarshal(trimmed, &decoded); err != nil {
		renderControlProxyData(c, status, map[string]any{})
		return
	}
	if obj, ok := decoded.(map[string]any); ok {
		if data, hasData := obj["data"]; hasData {
			if data == nil {
				renderControlProxyData(c, status, map[string]any{})
				return
			}
			renderControlProxyData(c, status, data)
			return
		}
		renderControlProxyData(c, status, map[string]any{})
		return
	}
	renderControlProxyData(c, status, decoded)
}

// renderControlProxyData renders one translated success body at the
// captured status. Every proxied legacy handler answers 200 today; the
// 201/202 branches keep status semantics unchanged if one ever does.
func renderControlProxyData(c *gin.Context, status int, data any) {
	switch status {
	case http.StatusCreated:
		contract.RenderCreated(c, data)
	case http.StatusAccepted:
		contract.RenderAccepted(c, data)
	default:
		contract.RenderSuccess(c, data)
	}
}

// renderControlProxyError reshapes a legacy error body into
// {error:{code,message,request_id}} at the same HTTP status.
// Machine-code bodies (string code: rewards/storage/search/statistics
// families) keep their wire-stable code verbatim; numeric-legacy bodies
// map by status to the kernel generic codes. request_id correlation comes
// from the same gin context via contract.RenderError.
func renderControlProxyError(c *gin.Context, status int, raw []byte) {
	var obj map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &obj); err != nil {
		obj = nil
	}
	message := controlProxyLegacyMessage(obj, status)
	if code, ok := obj["code"].(string); ok && strings.TrimSpace(code) != "" {
		contract.RenderError(c, status, contract.Code(strings.TrimSpace(code)), message)
		return
	}
	contract.RenderError(c, status, controlProxyNumericCode(status), message)
}

// controlProxyLegacyMessage keeps the legacy safe copy (msg or message)
// for translated errors; every legacy error here is already a display-safe
// string rendered through the response trust boundary.
func controlProxyLegacyMessage(obj map[string]any, status int) string {
	if obj != nil {
		if msg, ok := obj["msg"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
		if msg, ok := obj["message"].(string); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	return controlProxyDefaultMessage(status)
}

// controlProxyNumericCode maps a numeric-legacy error status to the
// stable kernel code. Statuses are replayed unchanged; only the code
// string is normalized.
func controlProxyNumericCode(status int) contract.Code {
	switch status {
	case http.StatusBadRequest:
		return contract.CodeBadRequest
	case http.StatusUnauthorized:
		return contract.CodeUnauthorized
	case http.StatusForbidden:
		return contract.CodeForbidden
	case http.StatusNotFound:
		return contract.CodeControlNotFound
	case http.StatusConflict:
		return contract.CodeConflict
	case http.StatusUnprocessableEntity:
		return contract.CodeValidationFailed
	case http.StatusTooManyRequests:
		return contract.CodeRateLimited
	case http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return contract.CodeServiceUnavailable
	default:
		if status >= 500 {
			return contract.CodeServiceUnavailable
		}
		return contract.CodeBadRequest
	}
}

// controlProxyDefaultMessage is the fallback copy when a legacy error
// body carries no message (unparsable or empty captures stay honest
// about the outcome without inventing domain detail).
func controlProxyDefaultMessage(status int) string {
	switch status {
	case http.StatusBadRequest:
		return msgBadRequest4
	case http.StatusUnauthorized:
		return msgUnauthorized
	case http.StatusForbidden:
		return msgForbidden
	case http.StatusNotFound:
		return msgControlProxyDefaultMessage
	case http.StatusConflict:
		return msgControlProxyDefaultMessage2
	case http.StatusTooManyRequests:
		return msgRateLimited2
	default:
		return msgServiceUnavailable24
	}
}
