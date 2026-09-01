package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestJWTAuthenticator(t *testing.T) {
	t.Parallel()

	secret := []byte("test-secret-at-least-32-chars-long!!")
	issuer := "https://api.example.com"
	auth := NewJWTAuthenticator(secret, issuer)

	t.Run("Generate and validate valid JWT token", func(t *testing.T) {
		token, err := auth.GenerateToken(JWTClaims{
			Issuer:    issuer,
			Subject:   "usr_123",
			Email:     "test@example.com",
			Roles:     []string{"admin", "user"},
			Scopes:    []string{"orders:read", "orders:write"},
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
			IssuedAt:  time.Now().Unix(),
		})
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		claims, err := auth.ValidateToken(token)
		if err != nil {
			t.Fatalf("failed to validate valid token: %v", err)
		}

		if claims.Subject != "usr_123" {
			t.Errorf("expected subject usr_123, got %s", claims.Subject)
		}
		if claims.Email != "test@example.com" {
			t.Errorf("expected email test@example.com, got %s", claims.Email)
		}
		if len(claims.Roles) != 2 || claims.Roles[0] != "admin" {
			t.Errorf("expected roles [admin, user], got %v", claims.Roles)
		}
	})

	t.Run("Expired token is rejected", func(t *testing.T) {
		token, err := auth.GenerateToken(JWTClaims{
			Issuer:    issuer,
			Subject:   "usr_expired",
			ExpiresAt: time.Now().Add(-1 * time.Hour).Unix(), // Expired 1 hour ago
		})
		if err != nil {
			t.Fatalf("failed to generate token: %v", err)
		}

		_, err = auth.ValidateToken(token)
		if err == nil || err.Error() != "token has expired" {
			t.Fatalf("expected 'token has expired' error, got %v", err)
		}
	})

	t.Run("Token signed with different secret is rejected", func(t *testing.T) {
		otherAuth := NewJWTAuthenticator([]byte("completely-different-secret-key-1234"), issuer)
		token, _ := otherAuth.GenerateToken(JWTClaims{
			Issuer:    issuer,
			Subject:   "usr_tampered",
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		})

		_, err := auth.ValidateToken(token)
		if err == nil || err.Error() != "invalid token signature" {
			t.Fatalf("expected 'invalid token signature' error, got %v", err)
		}
	})

	t.Run("Token with mismatched issuer is rejected", func(t *testing.T) {
		token, _ := auth.GenerateToken(JWTClaims{
			Issuer:    "https://evil.example.com",
			Subject:   "usr_wrong_issuer",
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		})

		_, err := auth.ValidateToken(token)
		if err == nil {
			t.Fatal("expected issuer validation error, got nil")
		}
	})
}

func TestAuthMiddlewareAndRBAC(t *testing.T) {
	t.Parallel()

	secret := []byte("test-secret-at-least-32-chars-long!!")
	jwtAuth := NewJWTAuthenticator(secret, "https://api.example.com")

	keyStore := NewAPIKeyStore(map[string]*AuthContext{
		"admin_api_key": {
			Subject:    "usr_admin_key",
			Email:      "admin@corp.com",
			Roles:      []string{"admin"},
			Scopes:     []string{"*"},
			AuthMethod: "api_key",
		},
		"readonly_api_key": {
			Subject:    "usr_readonly_key",
			Email:      "viewer@corp.com",
			Roles:      []string{"viewer"},
			Scopes:     []string{"orders:read"},
			AuthMethod: "api_key",
		},
	})

	// Protected handler requiring "admin" role and "orders:write" scope
	adminHandler := AuthMiddleware(jwtAuth, keyStore, false)(
		RequireRoles("admin")(
			RequireScopes("orders:write")(
				http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					user, _ := GetAuthUser(r.Context())
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("Hello " + user.Email))
				}),
			),
		),
	)

	t.Run("Missing credentials returns 401 Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		w := httptest.NewRecorder()
		adminHandler.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", w.Code)
		}
		if w.Header().Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header")
		}
	})

	t.Run("Valid Admin API Key succeeds", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("X-API-Key", "admin_api_key")
		w := httptest.NewRecorder()
		adminHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
		if w.Body.String() != "Hello admin@corp.com" {
			t.Errorf("expected body 'Hello admin@corp.com', got '%s'", w.Body.String())
		}
	})

	t.Run("Readonly API key fails with 403 Forbidden due to missing admin role", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("X-API-Key", "readonly_api_key")
		w := httptest.NewRecorder()
		adminHandler.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected status 403 Forbidden, got %d", w.Code)
		}
	})

	t.Run("Valid JWT Bearer token succeeds", func(t *testing.T) {
		token, _ := jwtAuth.GenerateToken(JWTClaims{
			Issuer:    "https://api.example.com",
			Subject:   "usr_jwt_admin",
			Email:     "jwt_admin@corp.com",
			Roles:     []string{"admin"},
			Scopes:    []string{"orders:read", "orders:write"},
			ExpiresAt: time.Now().Add(1 * time.Hour).Unix(),
		})

		req := httptest.NewRequest(http.MethodGet, "/admin", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		adminHandler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", w.Code)
		}
	})
}
