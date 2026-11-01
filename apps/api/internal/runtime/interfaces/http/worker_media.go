package http

import (
	"io"
	"net/http"
	"strings"

	"ideaboard/api/internal/media/imageformat"
	"ideaboard/api/internal/media/workspacemedia"
	platformworkermedia "ideaboard/api/internal/platform/workermedia"

	"github.com/gin-gonic/gin"
)

// WorkerMediaHandler serves the HMAC presigned media handoff for the sole
// Worker egress (ai-proxy): presigned GETs for inputs, one presigned
// PUT for the output. No session auth: every URL carries an HMAC signature
// binding method, exact key, content-type, and short expiry (see
// platform/workermedia). Binary endpoints carry no JSON envelope.
type WorkerMediaHandler struct {
	secret  string
	objects workspacemedia.MediaObjects
}

// NewWorkerMediaHandler builds the public HMAC media handler. Empty secret
// keeps every URL failing closed at verification time.
func NewWorkerMediaHandler(secret string, objects workspacemedia.MediaObjects) *WorkerMediaHandler {
	return &WorkerMediaHandler{secret: secret, objects: objects}
}

func registerWorkerMediaRoutes(r *gin.Engine, h *WorkerMediaHandler) {
	if h == nil {
		return
	}
	r.GET("/api/v1/worker/media/*key", h.ServeGET)
	r.PUT("/api/v1/worker/media/*key", h.ServePUT)
}

// workerMediaURL reconstructs the full presigned URL under verification.
func workerMediaURL(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	uri := strings.TrimSpace(c.Request.URL.RequestURI())
	if uri == "" {
		return ""
	}
	return uri
}

// ServeGET serves one presigned input object after HMAC verification.
func (h *WorkerMediaHandler) ServeGET(c *gin.Context) {
	if h == nil || h.objects == nil || strings.TrimSpace(h.secret) == "" {
		RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	verified, err := platformworkermedia.VerifyURL(h.secret, workerMediaURL(c), http.MethodGet)
	if err != nil {
		RenderV1BinaryError(c, http.StatusForbidden)
		return
	}
	object, err := h.objects.ReadObjectAtProfile(c.Request.Context(), workspacemedia.DefaultStorageProfile, verified.Key, workspacemedia.MaxUploadSizeBytes)
	if err != nil || object == nil || len(object.Data) == 0 {
		RenderV1BinaryError(c, http.StatusNotFound)
		return
	}
	serveType := strings.TrimSpace(object.ContentType)
	if serveType == "" {
		serveType = verified.ContentType
	}
	if serveType == "" {
		serveType = "application/octet-stream"
	}
	c.Header("Cache-Control", "no-store")
	c.Data(http.StatusOK, serveType, object.Data)
}

// ServePUT stores one presigned output object after HMAC verification.
// The signed binding is an exact supported MIME or SlotImagePolicy; both
// require matching MIME and a complete image. The key stays under the worker slot root.
func (h *WorkerMediaHandler) ServePUT(c *gin.Context) {
	if h == nil || h.objects == nil || strings.TrimSpace(h.secret) == "" {
		RenderV1BinaryError(c, http.StatusServiceUnavailable)
		return
	}
	verified, err := platformworkermedia.VerifyURL(h.secret, workerMediaURL(c), http.MethodPut)
	if err != nil {
		RenderV1BinaryError(c, http.StatusForbidden)
		return
	}
	if !platformworkermedia.ValidSlotKey(verified.Key) {
		RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	headerType := strings.TrimSpace(c.GetHeader("Content-Type"))
	if headerType == "" {
		headerType = strings.TrimSpace(c.ContentType())
	}
	headerType = strings.ToLower(headerType)
	if !platformworkermedia.AllowedOutputContentType(headerType) || (verified.ContentType != platformworkermedia.SlotImagePolicy && headerType != verified.ContentType) {
		RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, workspacemedia.MaxUploadSizeBytes+1))
	if err != nil || len(body) == 0 {
		RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if int64(len(body)) > workspacemedia.MaxUploadSizeBytes {
		RenderV1BinaryError(c, http.StatusRequestEntityTooLarge)
		return
	}
	facts, err := imageformat.InspectCompleteDetectedBytes(body)
	if err != nil || facts.MIMEType != headerType {
		RenderV1BinaryError(c, http.StatusBadRequest)
		return
	}
	if err := h.objects.PutObjectAtProfile(c.Request.Context(), workspacemedia.DefaultStorageProfile, verified.Key, body, facts.MIMEType); err != nil {
		RenderV1BinaryError(c, http.StatusBadGateway)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
}
