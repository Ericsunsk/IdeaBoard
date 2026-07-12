package jsonlimit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeStrictRequest(t *testing.T) {
	type requestBody struct {
		Name string `json:"name"`
	}

	tests := []struct {
		name    string
		body    string
		limit   int64
		wantErr bool
	}{
		{name: "single value", body: `{"name":"idea"}`, limit: 64},
		{name: "trailing whitespace", body: "{\"name\":\"idea\"}\n\t", limit: 64},
		{name: "unknown field", body: `{"name":"idea","extra":true}`, limit: 64, wantErr: true},
		{name: "multiple values", body: `{"name":"idea"} {"name":"board"}`, limit: 64, wantErr: true},
		{name: "over limit", body: `{"name":"idea"}`, limit: int64(len(`{"name":"idea"}`) - 1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var destination requestBody
			err := DecodeStrictRequest(recorder, request, tt.limit, &destination)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DecodeStrictRequest() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && destination.Name != "idea" {
				t.Fatalf("DecodeStrictRequest() destination = %+v", destination)
			}
		})
	}
}
