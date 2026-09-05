package httpapi

import (
	"bytes"
	"image"
	"image/png"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestValidateUploadFileHeaderCanonicalizesMismatchedKnownImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "remote.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if _, err := part.Write(imageBytes.Bytes()); err != nil {
		t.Fatalf("write avif header: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	file, err := context.FormFile("file")
	if err != nil {
		t.Fatalf("read form file: %v", err)
	}

	ext, ok := validateUploadFileHeader(context, file, 1024*1024, map[string]bool{".jpg": true, ".png": true})
	if !ok || ext != ".png" {
		t.Fatalf("ext = %q, ok = %v, status = %d, body = %s", ext, ok, recorder.Code, recorder.Body.String())
	}
}
