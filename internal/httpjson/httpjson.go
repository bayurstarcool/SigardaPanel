package httpjson

import (
	"encoding/json"
	"net/http"
)

type Success struct {
	Data      any    `json:"data"`
	RequestID string `json:"request_id"`
}

type ErrorBody struct {
	Error     APIError `json:"error"`
	RequestID string   `json:"request_id"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func Write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func OK(w http.ResponseWriter, requestID string, data any) {
	Write(w, http.StatusOK, Success{Data: data, RequestID: requestID})
}

func Error(w http.ResponseWriter, status int, requestID string, code string, message string) {
	Write(w, status, ErrorBody{
		Error:     APIError{Code: code, Message: message},
		RequestID: requestID,
	})
}
