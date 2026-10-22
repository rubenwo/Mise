package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// maxJSONBodyBytes caps JSON request bodies. Recipes, chats and pasted
// imports are all far below this.
const maxJSONBodyBytes = 2 << 20 // 2 MiB

// decodeJSON decodes the request body into dst, writing a 400 response and
// returning false when the body is missing, malformed, or too large.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

// pathID parses an integer URL parameter, writing a 400 response with msg
// and returning false when it is not a valid integer.
func pathID(w http.ResponseWriter, r *http.Request, param, msg string) (int, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, param))
	if err != nil {
		writeError(w, http.StatusBadRequest, msg)
		return 0, false
	}
	return id, true
}
