package httpx_test

import (
	"avito-kitchen/internal/platform/httpx"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

func testRouter() http.Handler {
	return httpx.NewRouter(slog.New(slog.DiscardHandler))
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	testRouter().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want ok", body.Status)
	}
}

func TestErrorEnvelope(t *testing.T) {
	tests := map[string]struct {
		method   string
		target   string
		status   int
		code     string
		incoming string
	}{
		"unknown path":       {http.MethodGet, "/nope", http.StatusNotFound, "not_found", ""},
		"wrong method":       {http.MethodPost, "/healthz", http.StatusMethodNotAllowed, "method_not_allowed", ""},
		"client's trace id":  {http.MethodGet, "/nope", http.StatusNotFound, "not_found", "trace-from-the-gateway"},
		"panicking handler":  {http.MethodGet, "/boom", http.StatusInternalServerError, "internal_error", ""},
		"panic keeps its id": {http.MethodGet, "/boom", http.StatusInternalServerError, "internal_error", "trace-42"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			router := httpx.NewRouter(slog.New(slog.DiscardHandler))
			router.Get("/boom", func(http.ResponseWriter, *http.Request) {
				panic("boom")
			})

			req := httptest.NewRequest(tt.method, tt.target, nil)
			if tt.incoming != "" {
				req.Header.Set(middleware.RequestIDHeader, tt.incoming)
			}

			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}

			raw, err := io.ReadAll(rec.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}

			var body httpx.ErrorBody
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Fatalf("decode body %q: %v", raw, err)
			}
			if body.Code != tt.code {
				t.Errorf("code = %q, want %q", body.Code, tt.code)
			}
			switch {
			case tt.incoming != "" && body.TraceID != tt.incoming:
				t.Errorf("trace_id = %q, want the client's %q", body.TraceID, tt.incoming)
			case body.TraceID == "":
				t.Error("trace_id is empty")
			}
		})
	}
}

func TestPanicIsLoggedWithTraceID(t *testing.T) {
	var logged strings.Builder
	router := httpx.NewRouter(slog.New(slog.NewJSONHandler(&logged, nil)))
	router.Get("/boom", func(http.ResponseWriter, *http.Request) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	req.Header.Set(middleware.RequestIDHeader, "trace-42")
	router.ServeHTTP(httptest.NewRecorder(), req)

	for _, want := range []string{"panic recovered", `"trace_id":"trace-42"`, `"stack"`, "request completed"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log does not contain %s\ngot: %s", want, logged.String())
		}
	}
}
