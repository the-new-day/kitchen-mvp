package kitchen

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/platform/httpx"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorHandlerResponse(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		err        error
		wantStatus int
		wantCode   string
		wantText   string
	}{
		"unknown venue": {
			err:        fmt.Errorf("get venue: %w", domain.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantCode:   "not_found",
			wantText:   "resource not found",
		},
		"bad argument keeps its message": {
			err:        domain.InvalidArgumentf("limit must be between 1 and 100"),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantText:   "limit must be between 1 and 100",
		},
		"operation of a later stage": {
			err:        errNotImplemented,
			wantStatus: http.StatusNotImplemented,
			wantCode:   "not_implemented",
		},
		"database is down": {
			err:        fmt.Errorf("query venues: %w", io.ErrUnexpectedEOF),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantText:   "internal server error",
		},
	}

	handler := errorHandler{log: slog.New(slog.DiscardHandler)}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/venues", nil)

			handler.response(recorder, request, tc.err)

			if recorder.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, tc.wantStatus)
			}

			var body httpx.ErrorBody
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body %q: %v", recorder.Body.String(), err)
			}

			if body.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", body.Code, tc.wantCode)
			}
			if tc.wantText != "" && body.Message != tc.wantText {
				t.Errorf("message = %q, want %q", body.Message, tc.wantText)
			}
		})
	}
}

func TestErrorHandlerRequest(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/venues/nope", nil)

	errorHandler{log: slog.New(slog.DiscardHandler)}.
		request(recorder, request, fmt.Errorf("invalid format for parameter venue_id"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
