package utils

import (
	"encoding/json"
	"net/http"
)

// ApiResponse wraps all API responses in the format the frontend expects.
type ApiResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}

// WriteJSON sends a success response.
func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ApiResponse{Success: true, Data: data})
}

// WriteError sends an error response.
func WriteError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ApiResponse{Success: false, Message: message})
}

// ParseFormValue extracts a form value from either FormData or URL-encoded form.
func ParseFormValue(r *http.Request, key string) string {
	if val := r.FormValue(key); val != "" {
		return val
	}
	// Also try multipart form
	if r.MultipartForm != nil {
		if v := r.MultipartForm.Value[key]; len(v) > 0 {
			return v[0]
		}
	}
	return ""
}
