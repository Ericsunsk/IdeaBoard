package taskservice

import (
	"context"

	"ideaboard/api/internal/tasking/taskmedia"
)

func validateSubmitTaskImageURLs(imageURLs []string) error {
	return validateSubmitTaskImageURLsWithRefValidator(imageURLs, validateSubmitTaskMediaRef)
}

func validateSubmitTaskMediaParams(params map[string]interface{}) error {
	return validateSubmitTaskMediaParamsWithRefValidator(params, validateSubmitTaskMediaRef)
}

func validateSubmitTaskMediaRef(raw string) error {
	return taskmedia.ValidateSubmitMediaRef(context.Background(), raw)
}
