package mcpserver

import (
	"encoding/json"
	"net/http"
)

// Health probes, matching sun-mcp: /livez (liveness + startup), /readyz
// (readiness) and /health (aggregate). The server needs no credentials and
// holds no state, so once it is serving it is ready.

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body) // a failed write to a dropped client is not actionable
}

func handleLivez(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func handleReadyz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"checks": map[string]string{"liveness": "ok", "readiness": "ok"},
	})
}
