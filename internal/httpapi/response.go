package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorResponse es el formato uniforme de error de la API.
type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

// ErrorBody contiene el código y el mensaje de error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON serializa v como JSON con el código de estado indicado.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("error serializando respuesta JSON", "error", err.Error())
	}
}

// writeError escribe una respuesta de error uniforme.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: message}})
}
