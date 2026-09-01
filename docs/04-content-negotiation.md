# Pattern 04: Content Negotiation & Media Types

## 1. Executive Summary & Specification Standards

**Content Negotiation (RFC 9110 Section 12)** is the mechanism by which a client and server agree on the optimal representation format for a resource.

In modern REST architectures:
- **Server-driven negotiation:** The client expresses preferences via standard HTTP request headers (`Accept`, `Accept-Language`, `Accept-Encoding`), and the server selects the best matching representation.
- **Client payload validation:** The server validates that incoming request payloads match supported MIME types via `Content-Type`.

---

## 2. The `Accept` Header & Quality Factors (`q`)

Clients can specify multiple acceptable media types with relative preference weights (quality values from `0.0` to `1.0`, defaulting to `1.0`):

```http
Accept: text/csv;q=0.9, application/json;q=1.0, application/xml;q=0.5, */*;q=0.1
```

In this example:
1. `application/json` is the first choice ($q = 1.0$).
2. `text/csv` is the second choice ($q = 0.9$).
3. `application/xml` is the third choice ($q = 0.5$).
4. Any remaining format ($*/*$) is the fallback ($q = 0.1$).

### Error Handling: 406 Not Acceptable
If the server cannot produce any representation matching the client's `Accept` header, it **MUST** return `406 Not Acceptable`:

```http
HTTP/1.1 406 Not Acceptable
Content-Type: application/problem+json

{
  "type": "https://api.example.com/errors/not-acceptable",
  "title": "Not Acceptable",
  "status": 406,
  "detail": "None of the requested media types in the Accept header are supported."
}
```

---

## 3. Request Payload Validation: 415 Unsupported Media Type

When a client submits data via `POST`, `PUT`, or `PATCH`, it must supply a valid `Content-Type` header (e.g. `application/json` or `application/x-www-form-urlencoded`).

If the client sends an unsupported format (e.g. `text/plain` or `application/yaml` to a JSON-only endpoint), the server **MUST** reject the request with `415 Unsupported Media Type`:

```http
HTTP/1.1 415 Unsupported Media Type
Content-Type: application/problem+json

{
  "type": "https://api.example.com/errors/unsupported-media-type",
  "title": "Unsupported Media Type",
  "status": 415,
  "detail": "Content-Type 'text/plain' is not supported. Allowed types: [application/json]"
}
```

---

## 4. Vendor Media Types & Content-Negotiated Versioning

Enterprise APIs frequently use custom vendor MIME types to version resources without polluting URL paths:

```http
Accept: application/vnd.company.orders.v2+json
```

### Parsing Vendor Media Types in Go

```go
func VersionFromVendorMediaType(acceptHeader string) string {
	for _, spec := range ParseAcceptHeader(acceptHeader) {
		if strings.HasPrefix(spec.Subtype, "vnd.") && strings.Contains(spec.Subtype, "+json") {
			middle := strings.TrimPrefix(spec.Subtype, "vnd.")
			middle = strings.TrimSuffix(middle, "+json")
			parts := strings.Split(middle, ".")
			if len(parts) >= 2 {
				return parts[len(parts)-1] // Extracts "v2"
			}
		}
	}
	return "v1"
}
```

---

## 5. Multi-Representation Renderer Architecture in Go

```go
func RenderResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	supported := []string{MediaTypeJSON, MediaTypeXML, MediaTypeCSV}
	negotiated := NegotiateContentType(r, supported)

	if negotiated == "" {
		NotAcceptable(w, r, "None of the requested media types in Accept header are supported.")
		return
	}

	switch negotiated {
	case MediaTypeXML:
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(xml.Header))
		_ = xml.NewEncoder(w).Encode(data)

	case MediaTypeCSV:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.WriteHeader(status)
		renderCSV(w, data)

	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(data)
	}
}
```
