package apierr_test

import (
	"avito-kitchen/internal/domain"
	"avito-kitchen/internal/platform/httpx"
	"avito-kitchen/internal/transport/http/apierr"
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
		err         error
		wantStatus  int
		wantCode    string
		wantText    string
		wantCurrent string
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
		"unknown api key": {
			err:        fmt.Errorf("authenticate: %w", domain.ErrUnauthenticated),
			wantStatus: http.StatusUnauthorized,
			wantCode:   "unauthorized",
		},
		"conflict keeps its own code": {
			err: fmt.Errorf("set item: %w",
				domain.Conflictf("cart_venue_conflict", "cart already holds items of another venue")),
			wantStatus: http.StatusConflict,
			wantCode:   "cart_venue_conflict",
			wantText:   "cart already holds items of another venue",
		},
		"below the minimum is not a conflict but a refusal": {
			err: fmt.Errorf("create order: %w",
				domain.Unprocessablef("below_minimum", "order is below the minimum")),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   "below_minimum",
			wantText:   "order is below the minimum",
		},
		"a forbidden transition of an order": {
			err:        fmt.Errorf("accept order: %w", domain.ErrInvalidTransition),
			wantStatus: http.StatusConflict,
			wantCode:   "invalid_transition",
		},
		"a forbidden transition reports the status the order is in": {
			err: fmt.Errorf("hand over order: %w",
				domain.InvalidTransition("ACCEPTED")),
			wantStatus:  http.StatusConflict,
			wantCode:    "invalid_transition",
			wantCurrent: "ACCEPTED",
		},
		"operation of a later stage": {
			err:        apierr.ErrNotImplemented,
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

	handler := apierr.Handler{Log: slog.New(slog.DiscardHandler)}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api/v1/venues", nil)

			handler.Response(recorder, request, tc.err)

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

			if tc.wantCurrent == "" {
				return
			}

			details, ok := body.Details.(map[string]any)
			if !ok || details["current_status"] != tc.wantCurrent {
				t.Errorf("details = %v, want current_status %q", body.Details, tc.wantCurrent)
			}
		})
	}
}

func TestErrorHandlerRequest(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/venues/nope", nil)

	apierr.Handler{Log: slog.New(slog.DiscardHandler)}.
		Request(recorder, request, fmt.Errorf("invalid format for parameter venue_id"))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
