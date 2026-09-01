package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProblemDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		callFunc       func(w http.ResponseWriter, r *http.Request)
		expectedStatus int
		expectedType   string
		expectedTitle  string
		checkHeader    func(t *testing.T, w *httptest.ResponseRecorder)
	}{
		{
			name: "BadRequest",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				BadRequest(w, r, "Invalid input data.", FieldError{Field: "email", Code: "INVALID"})
			},
			expectedStatus: http.StatusBadRequest,
			expectedType:   "https://api.example.com/errors/bad-request",
			expectedTitle:  "Bad Request",
		},
		{
			name: "NotFound",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				NotFound(w, r, "Order not found.")
			},
			expectedStatus: http.StatusNotFound,
			expectedType:   "https://api.example.com/errors/not-found",
			expectedTitle:  "Not Found",
		},
		{
			name: "Conflict",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				Conflict(w, r, "Resource conflict.")
			},
			expectedStatus: http.StatusConflict,
			expectedType:   "https://api.example.com/errors/conflict",
			expectedTitle:  "Conflict",
		},
		{
			name: "UnprocessableEntity",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				UnprocessableEntity(w, r, "Semantic validation failed.", FieldError{Field: "amount", Code: "POSITIVE"})
			},
			expectedStatus: http.StatusUnprocessableEntity,
			expectedType:   "https://api.example.com/errors/unprocessable-entity",
			expectedTitle:  "Unprocessable Entity",
		},
		{
			name: "TooManyRequests",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				TooManyRequests(w, r, 60)
			},
			expectedStatus: http.StatusTooManyRequests,
			expectedType:   "https://api.example.com/errors/rate-limited",
			expectedTitle:  "Too Many Requests",
			checkHeader: func(t *testing.T, w *httptest.ResponseRecorder) {
				if w.Header().Get("Retry-After") != "60" {
					t.Errorf("expected Retry-After 60, got %s", w.Header().Get("Retry-After"))
				}
			},
		},
		{
			name: "Unauthorized",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				Unauthorized(w, r, "Invalid authentication credentials.")
			},
			expectedStatus: http.StatusUnauthorized,
			expectedType:   "https://api.example.com/errors/unauthorized",
			expectedTitle:  "Unauthorized",
			checkHeader: func(t *testing.T, w *httptest.ResponseRecorder) {
				if w.Header().Get("WWW-Authenticate") == "" {
					t.Error("expected WWW-Authenticate header on 401 response")
				}
			},
		},
		{
			name: "Forbidden",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				Forbidden(w, r, "Insufficient permissions.")
			},
			expectedStatus: http.StatusForbidden,
			expectedType:   "https://api.example.com/errors/forbidden",
			expectedTitle:  "Forbidden",
		},
		{
			name: "NotAcceptable",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				NotAcceptable(w, r, "Media type not acceptable.")
			},
			expectedStatus: http.StatusNotAcceptable,
			expectedType:   "https://api.example.com/errors/not-acceptable",
			expectedTitle:  "Not Acceptable",
		},
		{
			name: "UnsupportedMediaType",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				UnsupportedMediaType(w, r, "Unsupported Content-Type.")
			},
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedType:   "https://api.example.com/errors/unsupported-media-type",
			expectedTitle:  "Unsupported Media Type",
		},
		{
			name: "PreconditionFailed",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				PreconditionFailed(w, r, "If-Match mismatch.")
			},
			expectedStatus: http.StatusPreconditionFailed,
			expectedType:   "https://api.example.com/errors/precondition-failed",
			expectedTitle:  "Precondition Failed",
		},
		{
			name: "PayloadTooLarge",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				PayloadTooLarge(w, r, "Payload size exceeds 1MB.")
			},
			expectedStatus: http.StatusRequestEntityTooLarge,
			expectedType:   "https://api.example.com/errors/payload-too-large",
			expectedTitle:  "Payload Too Large",
		},
		{
			name: "ServiceUnavailable",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				ServiceUnavailable(w, r, "Database maintenance.", 30)
			},
			expectedStatus: http.StatusServiceUnavailable,
			expectedType:   "https://api.example.com/errors/service-unavailable",
			expectedTitle:  "Service Unavailable",
			checkHeader: func(t *testing.T, w *httptest.ResponseRecorder) {
				if w.Header().Get("Retry-After") != "30" {
					t.Errorf("expected Retry-After 30, got %s", w.Header().Get("Retry-After"))
				}
			},
		},
		{
			name: "InternalError",
			callFunc: func(w http.ResponseWriter, r *http.Request) {
				InternalError(w, r)
			},
			expectedStatus: http.StatusInternalServerError,
			expectedType:   "https://api.example.com/errors/internal",
			expectedTitle:  "Internal Server Error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
			req = req.WithContext(WithAuthUser(req.Context(), &AuthContext{Subject: "test"}))
			w := httptest.NewRecorder()

			tt.callFunc(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if ct := w.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("expected Content-Type application/problem+json, got %s", ct)
			}

			var p Problem
			if err := json.NewDecoder(w.Body).Decode(&p); err != nil {
				t.Fatalf("failed to decode response body into Problem: %v", err)
			}

			if p.Status != tt.expectedStatus {
				t.Errorf("expected problem status %d, got %d", tt.expectedStatus, p.Status)
			}
			if p.Type != tt.expectedType {
				t.Errorf("expected problem type %s, got %s", tt.expectedType, p.Type)
			}
			if p.Title != tt.expectedTitle {
				t.Errorf("expected problem title %s, got %s", tt.expectedTitle, p.Title)
			}
			if p.Instance != "/test-path" {
				t.Errorf("expected instance /test-path, got %s", p.Instance)
			}

			if tt.checkHeader != nil {
				tt.checkHeader(t, w)
			}
		})
	}
}
