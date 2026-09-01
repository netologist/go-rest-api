# Pattern 15: OpenAPI 3.1 & API-First Contract Design

## 1. Executive Summary & The API-First Lifecycle

**API-First Design** is a software engineering methodology where the API specification (the machine-readable contract) is designed, reviewed, and finalized **before** writing implementation code.

```
Design Contract (OpenAPI 3.1) -> Architectural Review -> Generate Mock Server (Prism)
                                                                 |
               +-------------------------------------------------+
               |                                                 |
               ▼                                                 ▼
Frontend / Mobile Client Team                      Backend Engineering Team
(Build against Mock Server)                        (Implement Go Handlers & DB)
               |                                                 |
               +-------------------------------------------------+
                                       ▼
                       Automated Contract Testing (Pact / Dredd)
                                       ▼
                         Production Deployment & Portal
```

---

## 2. OpenAPI 3.1 Specifications Overview

OpenAPI 3.1 aligns fully with **JSON Schema 2020-12**, enabling precise modeling of polymorphism (`oneOf`, `anyOf`, `allOf`), type arrays (`["string", "null"]`), and Webhooks (`webhooks` top-level object).

### Core Components in OpenAPI 3.1

1. **Security Schemes:** Declarative authorization definitions (Bearer JWT, API Keys, OAuth2 flows).
2. **Schemas:** Domain entities (`Order`, `ProblemDetails`) with strict validation rules (regex patterns, min/max, enums).
3. **Paths:** Endpoints, HTTP methods, parameters (path, query, header), request bodies, and standardized response maps.
4. **Webhooks:** Declarative specifications of out-of-band events emitted by the API.

---

## 3. Interactive Documentation Integration in Go

The REST API exposes the live OpenAPI 3.1 document and an interactive Swagger UI interface directly over HTTP:

- **JSON Spec Endpoint:** `GET /openapi.json`
- **Swagger UI Interactive Explorer:** `GET /docs`

```go
// OpenAPISpecJSON serves the complete OpenAPI 3.1 specification
func ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(OpenAPISpecJSON))
}

// ServeSwaggerUI serves interactive Swagger UI HTML
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(SwaggerUIHTML))
}
```

---

## 4. Contract-Driven Testing in CI/CD

To guarantee that the backend implementation never drifts from the OpenAPI contract:

1. **Schema Validation Linters:** Run `spectral lint openapi.yaml` in CI to enforce naming conventions and descriptions.
2. **Contract Testing:** Use tools like **Schemathesis** or **Dredd** to automatically generate property-based HTTP test requests against the Go backend and verify that every response conforms to the OpenAPI schemas.
3. **Consumer-Driven Contracts (Pact):** Mobile and frontend teams write consumer expectations that run against backend pull requests before merging.
