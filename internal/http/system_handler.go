package http

import (
	"net/http"
	"time"

	"github.com/fanryan/paycore/internal/shared/httpjson"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func readyHandler(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func versionHandler(config RouterConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		httpjson.Write(w, http.StatusOK, map[string]any{
			"service":    config.ServiceName,
			"version":    config.Version,
			"started_at": config.StartedAt.UTC().Format(time.RFC3339),
		})
	}
}
