package httpx

import "net/http"

type healthBody struct {
	Status string `json:"status"`
}

// Healthz answers 200 while the process is running.
// It checks no dependencies.
func Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, healthBody{Status: "ok"})
	}
}
