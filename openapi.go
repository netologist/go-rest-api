package main

import (
	"net/http"
)

// SwaggerUIHTML provides an interactive browser documentation interface via Swagger UI CDN.
const SwaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>Production REST API Documentation</title>
  <link rel="stylesheet" type="text/css" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    html { box-sizing: border-box; overflow: -moz-scrollbars-vertical; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin:0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: "/openapi.json",
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "BaseLayout"
      });
    };
  </script>
</body>
</html>`

// OpenAPISpecJSON is the complete OpenAPI 3.1 specification for this API.
const OpenAPISpecJSON = `{
  "openapi": "3.1.0",
  "info": {
    "title": "Production-Grade REST API Blueprint",
    "version": "1.0.0",
    "description": "Comprehensive reference implementation of enterprise REST API design patterns in Go."
  },
  "servers": [
    { "url": "http://localhost:8080", "description": "Local Development Server" }
  ],
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT"
      },
      "ApiKeyAuth": {
        "type": "apiKey",
        "in": "header",
        "name": "X-API-Key"
      }
    },
    "schemas": {
      "Order": {
        "type": "object",
        "properties": {
          "id": { "type": "string", "example": "ord_1" },
          "status": { "type": "string", "enum": ["pending", "paid", "shipped", "cancelled", "refunded"], "example": "pending" },
          "amount": { "type": "number", "format": "double", "example": 149.99 },
          "currency": { "type": "string", "example": "USD" },
          "createdAt": { "type": "string", "format": "date-time" }
        },
        "required": ["id", "status", "amount", "currency", "createdAt"]
      },
      "ProblemDetails": {
        "type": "object",
        "properties": {
          "type": { "type": "string", "format": "uri" },
          "title": { "type": "string" },
          "status": { "type": "integer" },
          "detail": { "type": "string" },
          "instance": { "type": "string" },
          "requestId": { "type": "string" },
          "errors": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "field": { "type": "string" },
                "code": { "type": "string" }
              }
            }
          }
        },
        "required": ["type", "title", "status"]
      }
    }
  },
  "paths": {
    "/healthz": {
      "get": {
        "summary": "Liveness Probe",
        "responses": { "200": { "description": "Process is alive" } }
      }
    },
    "/readyz": {
      "get": {
        "summary": "Readiness Probe with Dependency Health",
        "responses": {
          "200": { "description": "All subsystems healthy" },
          "503": { "description": "One or more dependencies failing" }
        }
      }
    },
    "/metrics": {
      "get": {
        "summary": "Prometheus Metrics",
        "responses": { "200": { "description": "OpenMetrics/Prometheus format" } }
      }
    },
    "/api/v1/orders": {
      "get": {
        "summary": "List orders with cursor pagination, filtering, sorting, and sparse fieldsets",
        "parameters": [
          { "name": "limit", "in": "query", "schema": { "type": "integer", "default": 20 } },
          { "name": "cursor", "in": "query", "schema": { "type": "string" } },
          { "name": "sort", "in": "query", "schema": { "type": "string", "example": "-createdAt,amount" } },
          { "name": "status", "in": "query", "schema": { "type": "string" } },
          { "name": "fields", "in": "query", "schema": { "type": "string", "example": "id,status,amount" } }
        ],
        "responses": {
          "200": { "description": "List of orders" }
        }
      },
      "post": {
        "summary": "Create order (requires Idempotency-Key)",
        "security": [{ "BearerAuth": [] }, { "ApiKeyAuth": [] }],
        "parameters": [
          { "name": "Idempotency-Key", "in": "header", "required": true, "schema": { "type": "string" } }
        ],
        "responses": {
          "201": { "description": "Order created successfully" },
          "422": { "description": "Validation error" }
        }
      }
    },
    "/api/v1/jobs/export": {
      "post": {
        "summary": "Trigger asynchronous orders export (202 Accepted)",
        "responses": {
          "202": { "description": "Accepted for processing, returns Location header" }
        }
      }
    },
    "/api/v1/jobs/{id}": {
      "get": {
        "summary": "Poll async job status",
        "parameters": [{ "name": "id", "in": "path", "required": true, "schema": { "type": "string" } }],
        "responses": {
          "200": { "description": "Job status and progress" }
        }
      }
    }
  }
}`

// ServeOpenAPISpec returns the raw OpenAPI 3.1 JSON document.
func ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(OpenAPISpecJSON))
}

// ServeSwaggerUI serves interactive Swagger UI documentation.
func ServeSwaggerUI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(SwaggerUIHTML))
}
