# Pattern 02: Authentication & Authorization (AuthN & AuthZ)

## 1. Authentication vs Authorization in REST APIs

In enterprise REST architectures:
- **Authentication (AuthN):** Establishes and verifies *who* the caller is (identity verification via JWT, mTLS, or API Keys).
- **Authorization (AuthZ):** Evaluates *what* permissions the verified identity possesses (RBAC roles, ABAC attributes, OAuth2 scopes).

```
Incoming Request -> [AuthN: Validate JWT / API Key] -> [AuthZ: Check Roles / Scopes] -> Handler
                           |                                     |
                     Invalid? -> 401                       Forbidden? -> 403
```

---

## 2. HTTP Status Code Semantics: 401 vs 403

| Status Code | Meaning | RFC Requirement |
|---|---|---|
| **401 Unauthorized** | The caller is unauthenticated or supplied invalid/expired credentials. | **MUST** include the `WWW-Authenticate` header (RFC 7235). |
| **403 Forbidden** | The caller is authenticated, but their identity lacks the necessary permissions/roles for this resource. | No `WWW-Authenticate` header. Re-authenticating with the same credentials will not help. |

---

## 3. Credential Types & Security Strategy

### 1. JSON Web Tokens (JWT Bearer — RFC 7519)
- **Use Case:** Single Sign-On (SSO), frontend-to-backend calls, user identity propagation across microservices.
- **Header:** `Authorization: Bearer <jwt-token>`
- **Security Invariants:**
  - Verify signature with a cryptographic secret (HS256) or asymmetric key pair (RS256 / Ed25519).
  - Enforce `exp` (expiration), `nbf` (not before), and `iss` (issuer).
  - Never store sensitive data (passwords, PII) in unencrypted JWT claims.

### 2. API Keys
- **Use Case:** Machine-to-machine (M2M) server communications, third-party partner integrations.
- **Header:** `X-API-Key: <opaque-key>` or `Authorization: ApiKey <opaque-key>`
- **Security Invariants:**
  - Keys should be high-entropy random strings (at least 256 bits).
  - Store salted hashes (e.g. SHA-256 or bcrypt) in the database, never plaintext keys.
  - Implement instant key revocation and rotation capabilities.

---

## 4. Role-Based & Scope-Based Access Control

### Role-Based Access Control (RBAC)
Maps identities to coarse-grained operational roles: `admin`, `manager`, `user`, `auditor`.

```go
// Enforce admin role
r.With(
    AuthMiddleware(jwtAuth, keyStore, false),
    RequireRoles("admin"),
).Get("/admin/stats", adminHandler)
```

### OAuth2 Scopes
Fine-grained, resource-specific capabilities: `orders:read`, `orders:write`, `payments:refund`.

```go
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
```

---

## 5. Security Best Practices & OWASP Hardening

1. **Constant-Time Comparison:** When validating HMAC signatures or API keys, always use `crypto/subtle.ConstantTimeCompare` or `hmac.Equal` to prevent side-channel timing attacks.
2. **Short-Lived Access Tokens:** Issue access tokens with 15–60 minute lifetimes, paired with refresh tokens stored in secure, `HttpOnly`, `SameSite=Strict` cookies or distributed revocation stores.
3. **Context Injection:** Once validated, store an immutable `*AuthContext` in `r.Context()`. Downstream business logic should never re-parse HTTP headers.
