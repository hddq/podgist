package http

import (
	"encoding/json"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		json.NewEncoder(w).Encode(v)
	}
}

func readJSON(r *http.Request, maxSize int, v any) error {
	body := io.LimitReader(r.Body, int64(maxSize))
	return json.NewDecoder(body).Decode(v)
}
