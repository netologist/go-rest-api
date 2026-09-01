package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// AuthContext holds identity and authorization attributes extracted from
// a validated credential (JWT, API key, or OAuth2 access token).
type AuthContext struct {
	Subject    string   `json:"sub"`
	Email      string   `json:"email,omitempty"`
	Roles      []string `json:"roles"`
	Scopes     []string `json:"scopes"`
	ClientID   string   `json:"clientId,omitempty"`
	AuthMethod string   `json:"authMethod"` // "bearer" or "api_key"
}

// HasRole checks if the authenticated identity contains the given role.
func (a *AuthContext) HasRole(role string) bool {
	for _, r := range a.Roles {
		if strings.EqualFold(r, role) {
			return true
		}
	}
	return false
}

// HasScope checks if the authenticated identity possesses the specified scope.
func (a *AuthContext) HasScope(scope string) bool {
	for _, s := range a.Scopes {
		if s == scope || s == "*" {
			return true
		}
	}
	return false
}

type authCtxKey struct{}

// WithAuthUser stores the AuthContext inside the request context.
func WithAuthUser(ctx context.Context, auth *AuthContext) context.Context {
	return context.WithValue(ctx, authCtxKey{}, auth)
}

// GetAuthUser retrieves the authenticated identity from the context.
func GetAuthUser(ctx context.Context) (*AuthContext, bool) {
	v, ok := ctx.Value(authCtxKey{}).(*AuthContext)
	return v, ok && v != nil
}

// JWTClaims represents standard and custom claims contained in a JWT token.
type JWTClaims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  string   `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	IssuedAt  int64    `json:"iat"`
	Roles     []string `json:"roles"`
	Scopes    []string `json:"scopes"`
	Email     string   `json:"email"`
}

// JWTAuthenticator parses and verifies HMAC-SHA256 signed JSON Web Tokens.
type JWTAuthenticator struct {
	secretKey []byte
	issuer    string
}

// NewJWTAuthenticator creates a new JWT authenticator instance.
func NewJWTAuthenticator(secretKey []byte, issuer string) *JWTAuthenticator {
	return &JWTAuthenticator{secretKey: secretKey, issuer: issuer}
}

// GenerateToken creates a signed HS256 JWT token with the given claims (useful for tests & auth endpoints).
func (j *JWTAuthenticator) GenerateToken(claims JWTClaims) (string, error) {
	headerJSON, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)
	unsigned := headerB64 + "." + claimsB64

	mac := hmac.New(sha256.New, j.secretKey)
	mac.Write([]byte(unsigned))
	sigB64 := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return unsigned + "." + sigB64, nil
}

// ValidateToken verifies signature, expiry, and issuer, returning parsed claims.
func (j *JWTAuthenticator) ValidateToken(tokenStr string) (*JWTClaims, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed JWT: must contain header, payload, and signature")
	}

	unsigned := parts[0] + "." + parts[1]
	expectedSig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid signature encoding")
	}

	mac := hmac.New(sha256.New, j.secretKey)
	mac.Write([]byte(unsigned))
	actualSig := mac.Sum(nil)

	if !hmac.Equal(expectedSig, actualSig) {
		return nil, errors.New("invalid token signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("invalid payload encoding")
	}

	var claims JWTClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, fmt.Errorf("unmarshal claims: %w", err)
	}

	now := time.Now().Unix()
	if claims.ExpiresAt > 0 && now > claims.ExpiresAt {
		return nil, errors.New("token has expired")
	}
	if j.issuer != "" && claims.Issuer != j.issuer {
		return nil, fmt.Errorf("invalid issuer: expected %s, got %s", j.issuer, claims.Issuer)
	}

	return &claims, nil
}

// APIKeyStore validates static or dynamically assigned API keys.
type APIKeyStore struct {
	keys map[string]*AuthContext
}

// NewAPIKeyStore initializes a key store with predefined keys.
func NewAPIKeyStore(keys map[string]*AuthContext) *APIKeyStore {
	return &APIKeyStore{keys: keys}
}

// Lookup retrieves identity associated with the API key.
func (s *APIKeyStore) Lookup(key string) (*AuthContext, bool) {
	ctx, ok := s.keys[key]
	return ctx, ok
}

// AuthMiddleware inspects Authorization header (Bearer token) or X-API-Key header.
func AuthMiddleware(jwtAuth *JWTAuthenticator, keyStore *APIKeyStore, optional bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			apiKeyHeader := r.Header.Get("X-API-Key")

			if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
				token := strings.TrimPrefix(authHeader, "Bearer ")
				claims, err := jwtAuth.ValidateToken(token)
				if err != nil {
					Unauthorized(w, r, "Invalid or expired Bearer token: "+err.Error())
					return
				}

				authCtx := &AuthContext{
					Subject:    claims.Subject,
					Email:      claims.Email,
					Roles:      claims.Roles,
					Scopes:     claims.Scopes,
					AuthMethod: "bearer",
				}
				next.ServeHTTP(w, r.WithContext(WithAuthUser(r.Context(), authCtx)))
				return
			}

			if apiKeyHeader != "" && keyStore != nil {
				if authCtx, ok := keyStore.Lookup(apiKeyHeader); ok {
					next.ServeHTTP(w, r.WithContext(WithAuthUser(r.Context(), authCtx)))
					return
				}
				Unauthorized(w, r, "Invalid API key provided.")
				return
			}

			if optional {
				next.ServeHTTP(w, r)
				return
			}

			Unauthorized(w, r, "Missing Authorization header or X-API-Key.")
		})
	}
}

// RequireRoles enforces Role-Based Access Control (RBAC). The caller must have at least one of the listed roles.
func RequireRoles(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetAuthUser(r.Context())
			if !ok {
				Unauthorized(w, r, "Authentication required.")
				return
			}

			for _, role := range roles {
				if user.HasRole(role) {
					next.ServeHTTP(w, r)
					return
				}
			}

			Forbidden(w, r, fmt.Sprintf("Access denied: required role in %v", roles))
		})
	}
}

// RequireScopes enforces OAuth2-style fine-grained scope authorization.
func RequireScopes(requiredScopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetAuthUser(r.Context())
			if !ok {
				Unauthorized(w, r, "Authentication required.")
				return
			}

			for _, scope := range requiredScopes {
				if !user.HasScope(scope) {
					Forbidden(w, r, fmt.Sprintf("Access denied: missing required scope '%s'", scope))
					return
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
