package web

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type statsResponse struct {
	TotalRuns   int         `json:"total_Runs"`
	ActiveRuns  int         `json:"active_Runs"`
	SuccessRate float64     `json:"success_rate"`
	AvgDuration string      `json:"avg_duration"`
	TokensUsed  tokensUsage `json:"tokens_used"`
}

type tokensUsage struct {
	Claude int `json:"claude"`
	Codex  int `json:"codex"`
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleStats(w http.ResponseWriter, _ *http.Request) {
	resp := statsResponse{
		TotalRuns:   0,
		ActiveRuns:  0,
		SuccessRate: 0,
		AvgDuration: "0s",
		TokensUsed: tokensUsage{
			Claude: 0,
			Codex:  0,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body.Bytes())
}
