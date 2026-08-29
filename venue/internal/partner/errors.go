package partner

import (
	"avito-kitchen/internal/api/partnerapi"
	"avito-kitchen/venue/internal/kitchen"
	"encoding/json"
	"fmt"
	"net/http"
)

// check turns the answer of the platform into an error of the kitchen.
func check(what string, status int, body []byte) error {
	if status >= http.StatusOK && status < http.StatusMultipleChoices {
		return nil
	}

	failure := decode(body)

	switch status {
	case http.StatusUnauthorized:
		return fmt.Errorf("%s: %w", what, kitchen.ErrUnauthorized)
	case http.StatusNotFound:
		return fmt.Errorf("%s: %w", what, kitchen.ErrNotFound)
	case http.StatusConflict:
		return fmt.Errorf("%s: %w", what, kitchen.RefusedError{Current: currentStatus(failure)})
	default:
		return fmt.Errorf("%s: platform answered %d %s: %s",
			what, status, failure.Code, failure.Message)
	}
}

// decode reads the error envelope of the platform, tolerating a body that is
// not one: the status code is the part that matters.
func decode(body []byte) partnerapi.Error {
	var failure partnerapi.Error

	if err := json.Unmarshal(body, &failure); err != nil {
		return partnerapi.Error{Code: "unreadable", Message: string(body)}
	}

	return failure
}

// currentStatus reads the status an order turned out to be in.
func currentStatus(failure partnerapi.Error) string {
	if failure.Details == nil {
		return ""
	}

	current, ok := (*failure.Details)["current_status"].(string)
	if !ok {
		return ""
	}

	return current
}
