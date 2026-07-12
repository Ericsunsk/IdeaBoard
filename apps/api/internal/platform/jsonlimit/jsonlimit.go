package jsonlimit

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

var errMultipleJSONValues = errors.New("request body must contain one JSON value")

// DecodeStrictRequest bounds and decodes one JSON value while rejecting
// unknown fields and any additional non-whitespace content.
func DecodeStrictRequest(w http.ResponseWriter, request *http.Request, maxBytes int64, destination any) error {
	request.Body = http.MaxBytesReader(w, request.Body, maxBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errMultipleJSONValues
	}
	return nil
}

func ValueDepth(value interface{}) int {
	switch v := value.(type) {
	case map[string]interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + ValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	case []interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + ValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	default:
		return 1
	}
}
