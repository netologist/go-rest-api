package main

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// Problem implements the RFC 7807 (Problem Details for HTTP APIs) format.
type Problem struct {
	Type      string       `json:"type"`
	Title     string       `json:"title"`
	Status    int          `json:"status"`
	Detail    string       `json:"detail,omitempty"`
	Instance  string       `json:"instance,omitempty"`
	RequestID string       `json:"requestId,omitempty"`
	Errors    []FieldError `json:"errors,omitempty"`
}

// FieldError represents a single field-level validation error.
type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"`
}

// WriteProblem writes a standard error response and sets the Content-Type
// to application/problem+json, per RFC 7807.
func WriteProblem(w http.ResponseWriter, r *http.Request, status int, problemType, title, detail string, fieldErrors ...FieldError) {
	p := Problem{
		Type:      problemType,
		Title:     title,
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		RequestID: GetRequestID(r.Context()),
		Errors:    fieldErrors,
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(p)
}

// BadRequest: the request could not be parsed/understood (400).
func BadRequest(w http.ResponseWriter, r *http.Request, detail string, fieldErrors ...FieldError) {
	WriteProblem(w, r, http.StatusBadRequest, "https://api.example.com/errors/bad-request", "Bad Request", detail, fieldErrors...)
}

// NotFound: the resource does not exist (404).
func NotFound(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusNotFound, "https://api.example.com/errors/not-found", "Not Found", detail)
}

// Conflict: a state conflict, e.g. concurrent idempotent request in-flight (409).
func Conflict(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusConflict, "https://api.example.com/errors/conflict", "Conflict", detail)
}

// UnprocessableEntity: syntactically valid but semantically invalid (422).
func UnprocessableEntity(w http.ResponseWriter, r *http.Request, detail string, fieldErrors ...FieldError) {
	WriteProblem(w, r, http.StatusUnprocessableEntity, "https://api.example.com/errors/unprocessable-entity", "Unprocessable Entity", detail, fieldErrors...)
}

// TooManyRequests: rate limit exceeded (429), always paired with Retry-After.
func TooManyRequests(w http.ResponseWriter, r *http.Request, retryAfterSeconds int) {
	w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	WriteProblem(w, r, http.StatusTooManyRequests, "https://api.example.com/errors/rate-limited", "Too Many Requests",
		"Rate limit exceeded, please retry after the specified duration.")
}

// InternalError: an unexpected server-side failure (500).
// Never leak stack traces or internal error details to the client.
func InternalError(w http.ResponseWriter, r *http.Request) {
	WriteProblem(w, r, http.StatusInternalServerError, "https://api.example.com/errors/internal", "Internal Server Error",
		"An unexpected error occurred.")
}

// Unauthorized: authentication is required or has failed (401).
// RFC 7235 requires sending a WWW-Authenticate header with 401.
func Unauthorized(w http.ResponseWriter, r *http.Request, detail string) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="api.example.com", error="invalid_token"`)
	WriteProblem(w, r, http.StatusUnauthorized, "https://api.example.com/errors/unauthorized", "Unauthorized", detail)
}

// Forbidden: client is authenticated but lacks permission for this action (403).
func Forbidden(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusForbidden, "https://api.example.com/errors/forbidden", "Forbidden", detail)
}

// NotAcceptable: the server cannot produce a response matching the Accept header (406).
func NotAcceptable(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusNotAcceptable, "https://api.example.com/errors/not-acceptable", "Not Acceptable", detail)
}

// UnsupportedMediaType: the request payload is in a format not supported by this endpoint (415).
func UnsupportedMediaType(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusUnsupportedMediaType, "https://api.example.com/errors/unsupported-media-type", "Unsupported Media Type", detail)
}

// PreconditionFailed: conditional request precondition (e.g. If-Match) evaluated to false (412).
func PreconditionFailed(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusPreconditionFailed, "https://api.example.com/errors/precondition-failed", "Precondition Failed", detail)
}

// PayloadTooLarge: request body exceeds maximum allowed size (413).
func PayloadTooLarge(w http.ResponseWriter, r *http.Request, detail string) {
	WriteProblem(w, r, http.StatusRequestEntityTooLarge, "https://api.example.com/errors/payload-too-large", "Payload Too Large", detail)
}

// ServiceUnavailable: temporary server-side inability to handle the request (503).
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, detail string, retryAfterSeconds int) {
	if retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
	}
	WriteProblem(w, r, http.StatusServiceUnavailable, "https://api.example.com/errors/service-unavailable", "Service Unavailable", detail)
}
