package httpx

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// ErrorBody is the body of every error response.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
	TraceID string `json:"trace_id,omitempty"`
}

// WriteJSON sends v as the response body with the given status. It panics if
// v cannot be marshalled, before anything reaches the client.
// A failed write is ignored.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Errorf("marshal response body: %w", err))
	}
	body = append(body, '\n')

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// WriteError sends an ErrorBody with the given status, tagged with the trace
// id of the request.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	WriteErrorDetails(w, r, status, code, message, nil)
}

// WriteErrorDetails sends an ErrorBody carrying what the client needs to act on
// the error: the status an entity is really in, the field that failed.
func WriteErrorDetails(
	w http.ResponseWriter, r *http.Request, status int, code, message string, details any,
) {
	WriteJSON(w, status, ErrorBody{
		Code:    code,
		Message: message,
		Details: details,
		TraceID: middleware.GetReqID(r.Context()),
	})
}
