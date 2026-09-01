package main

import (
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Supported standard media types
const (
	MediaTypeJSON       = "application/json"
	MediaTypeXML        = "application/xml"
	MediaTypeTextXML    = "text/xml"
	MediaTypeCSV        = "text/csv"
	MediaTypeProblemJSON = "application/problem+json"
)

// AcceptSpec represents a parsed entry in the HTTP Accept header with its quality factor.
type AcceptSpec struct {
	Type     string
	Subtype  string
	FullType string
	Quality  float64
	Params   map[string]string
}

// ParseAcceptHeader parses the RFC 9110 Accept header and returns specs ordered by quality (descending).
func ParseAcceptHeader(header string) []AcceptSpec {
	if strings.TrimSpace(header) == "" {
		return []AcceptSpec{{Type: "*", Subtype: "*", FullType: "*/*", Quality: 1.0, Params: make(map[string]string)}}
	}

	var specs []AcceptSpec
	rawEntries := strings.Split(header, ",")

	for _, raw := range rawEntries {
		parts := strings.Split(strings.TrimSpace(raw), ";")
		if len(parts) == 0 || parts[0] == "" {
			continue
		}

		mimeParts := strings.SplitN(strings.TrimSpace(parts[0]), "/", 2)
		if len(mimeParts) != 2 {
			continue
		}

		spec := AcceptSpec{
			Type:     strings.ToLower(strings.TrimSpace(mimeParts[0])),
			Subtype:  strings.ToLower(strings.TrimSpace(mimeParts[1])),
			FullType: strings.ToLower(strings.TrimSpace(parts[0])),
			Quality:  1.0,
			Params:   make(map[string]string),
		}

		for _, p := range parts[1:] {
			kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
			if len(kv) == 2 {
				k := strings.ToLower(strings.TrimSpace(kv[0]))
				v := strings.Trim(strings.TrimSpace(kv[1]), "\"")
				if k == "q" {
					if qVal, err := strconv.ParseFloat(v, 64); err == nil && qVal >= 0.0 && qVal <= 1.0 {
						spec.Quality = qVal
					}
				} else {
					spec.Params[k] = v
				}
			}
		}
		specs = append(specs, spec)
	}

	sort.SliceStable(specs, func(i, j int) bool {
		return specs[i].Quality > specs[j].Quality
	})

	return specs
}

// NegotiateContentType selects the best match between client Accept preferences and supported server types.
func NegotiateContentType(r *http.Request, supported []string) string {
	acceptHeader := r.Header.Get("Accept")
	specs := ParseAcceptHeader(acceptHeader)

	for _, spec := range specs {
		if spec.Quality <= 0 {
			continue
		}
		if spec.Type == "*" && spec.Subtype == "*" {
			if len(supported) > 0 {
				return supported[0]
			}
			return MediaTypeJSON
		}

		for _, supp := range supported {
			suppParts := strings.SplitN(supp, "/", 2)
			if len(suppParts) != 2 {
				continue
			}
			suppType, suppSubtype := suppParts[0], suppParts[1]

			if (spec.Type == suppType || spec.Type == "*") &&
				(spec.Subtype == suppSubtype || spec.Subtype == "*") {
				return supp
			}
		}
	}

	return ""
}

// RenderResponse writes data in the format negotiated via Accept header.
func RenderResponse(w http.ResponseWriter, r *http.Request, status int, data any) {
	supported := []string{MediaTypeJSON, MediaTypeXML, MediaTypeTextXML, MediaTypeCSV}
	negotiated := NegotiateContentType(r, supported)

	if negotiated == "" {
		NotAcceptable(w, r, "None of the requested media types in Accept header are supported.")
		return
	}

	switch negotiated {
	case MediaTypeXML, MediaTypeTextXML:
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(xml.Header))
		_ = xml.NewEncoder(w).Encode(data)

	case MediaTypeCSV:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.WriteHeader(status)
		renderCSV(w, data)

	case MediaTypeJSON:
		fallthrough
	default:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(data)
	}
}

// renderCSV converts structs or slices to tabular CSV format.
func renderCSV(w http.ResponseWriter, data any) {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	switch v := data.(type) {
	case Order:
		_ = cw.Write([]string{"ID", "Status", "Amount", "Currency", "CreatedAt"})
		_ = cw.Write([]string{v.ID, v.Status, fmt.Sprintf("%.2f", v.Amount), v.Currency, v.CreatedAt})
	case []Order:
		_ = cw.Write([]string{"ID", "Status", "Amount", "Currency", "CreatedAt"})
		for _, o := range v {
			_ = cw.Write([]string{o.ID, o.Status, fmt.Sprintf("%.2f", o.Amount), o.Currency, o.CreatedAt})
		}
	default:
		_ = cw.Write([]string{"Data"})
		_ = cw.Write([]string{fmt.Sprintf("%v", data)})
	}
}

// ValidateContentTypeMiddleware enforces that request bodies (for POST/PUT/PATCH) match accepted MIME types.
func ValidateContentTypeMiddleware(allowedTypes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			// If no body is provided (Content-Length is 0), skip validation
			if r.ContentLength == 0 && r.Body == http.NoBody {
				next.ServeHTTP(w, r)
				return
			}

			ct := r.Header.Get("Content-Type")
			if ct == "" {
				UnsupportedMediaType(w, r, "The 'Content-Type' header is required for request payloads.")
				return
			}

			// Extract media type without charset/boundary parameters
			mediaType := strings.ToLower(strings.TrimSpace(strings.Split(ct, ";")[0]))
			for _, allowed := range allowedTypes {
				if strings.EqualFold(mediaType, allowed) {
					next.ServeHTTP(w, r)
					return
				}
			}

			UnsupportedMediaType(w, r, fmt.Sprintf("Content-Type '%s' is not supported. Allowed types: %v", ct, allowedTypes))
		})
	}
}

// VersionFromVendorMediaType extracts API version from vendor media type, e.g. application/vnd.example.v2+json -> "v2"
func VersionFromVendorMediaType(acceptHeader string) string {
	for _, spec := range ParseAcceptHeader(acceptHeader) {
		if strings.HasPrefix(spec.Subtype, "vnd.") && strings.Contains(spec.Subtype, "+json") {
			// Extract part between vnd. and +json
			middle := strings.TrimPrefix(spec.Subtype, "vnd.")
			middle = strings.TrimSuffix(middle, "+json")
			parts := strings.Split(middle, ".")
			if len(parts) >= 2 {
				return parts[len(parts)-1] // e.g. "v2"
			}
		}
	}
	return ""
}
