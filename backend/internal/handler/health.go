package handler

import (
	"encoding/json"
	"net/http"
)

type healthResponse struct {
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks"`
}

func HealthCheck(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status: "ok",
		Checks: map[string]string{
			"server": "ok",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}
