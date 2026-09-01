package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseAcceptHeaderAndNegotiation(t *testing.T) {
	t.Parallel()

	t.Run("Quality factor priority is respected", func(t *testing.T) {
		header := "text/html, application/xhtml+xml, application/xml;q=0.9, application/json;q=0.8, */*;q=0.1"
		specs := ParseAcceptHeader(header)

		if len(specs) != 5 {
			t.Fatalf("expected 5 specs, got %d", len(specs))
		}
		if specs[0].FullType != "text/html" || specs[0].Quality != 1.0 {
			t.Errorf("highest priority expected text/html (q=1.0), got %v", specs[0])
		}
		if specs[len(specs)-1].FullType != "*/*" || specs[len(specs)-1].Quality != 0.1 {
			t.Errorf("lowest priority expected */* (q=0.1), got %v", specs[len(specs)-1])
		}
	})

	t.Run("NegotiateContentType selects supported type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/csv;q=0.9, application/json;q=1.0")

		supported := []string{MediaTypeJSON, MediaTypeCSV, MediaTypeXML}
		selected := NegotiateContentType(req, supported)

		if selected != MediaTypeJSON {
			t.Errorf("expected application/json, got %s", selected)
		}
	})

	t.Run("RenderResponse correctly outputs JSON, XML, CSV", func(t *testing.T) {
		order := Order{
			ID:        "ord_test",
			Status:    "paid",
			Amount:    99.90,
			Currency:  "USD",
			CreatedAt: "2026-08-29T12:00:00Z",
		}

		// JSON request
		reqJSON := httptest.NewRequest(http.MethodGet, "/", nil)
		reqJSON.Header.Set("Accept", "application/json")
		wJSON := httptest.NewRecorder()
		RenderResponse(wJSON, reqJSON, http.StatusOK, order)
		if !strings.Contains(wJSON.Header().Get("Content-Type"), "application/json") {
			t.Errorf("expected application/json Content-Type, got %s", wJSON.Header().Get("Content-Type"))
		}
		if !strings.Contains(wJSON.Body.String(), `"id":"ord_test"`) {
			t.Errorf("expected JSON body with order ID, got %s", wJSON.Body.String())
		}

		// XML request
		reqXML := httptest.NewRequest(http.MethodGet, "/", nil)
		reqXML.Header.Set("Accept", "application/xml")
		wXML := httptest.NewRecorder()
		RenderResponse(wXML, reqXML, http.StatusOK, order)
		if !strings.Contains(wXML.Header().Get("Content-Type"), "application/xml") {
			t.Errorf("expected application/xml Content-Type, got %s", wXML.Header().Get("Content-Type"))
		}
		if !strings.Contains(wXML.Body.String(), "<Order>") || !strings.Contains(wXML.Body.String(), "<id>ord_test</id>") {
			t.Errorf("expected XML body with <Order>, got %s", wXML.Body.String())
		}

		// CSV request
		reqCSV := httptest.NewRequest(http.MethodGet, "/", nil)
		reqCSV.Header.Set("Accept", "text/csv")
		wCSV := httptest.NewRecorder()
		RenderResponse(wCSV, reqCSV, http.StatusOK, order)
		if !strings.Contains(wCSV.Header().Get("Content-Type"), "text/csv") {
			t.Errorf("expected text/csv Content-Type, got %s", wCSV.Header().Get("Content-Type"))
		}
		if !strings.Contains(wCSV.Body.String(), "ID,Status,Amount,Currency,CreatedAt") || !strings.Contains(wCSV.Body.String(), "ord_test,paid,99.90,USD") {
			t.Errorf("expected CSV body, got %s", wCSV.Body.String())
		}
	})

	t.Run("ValidateContentTypeMiddleware rejects invalid content type on POST with 415", func(t *testing.T) {
		mw := ValidateContentTypeMiddleware(MediaTypeJSON)
		nextCalled := false
		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`plain text body`))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		if w.Code != http.StatusUnsupportedMediaType {
			t.Fatalf("expected status 415 Unsupported Media Type, got %d", w.Code)
		}
		if nextCalled {
			t.Error("downstream handler should not have been called on 415")
		}
	})

	t.Run("Vendor media type version extraction", func(t *testing.T) {
		accept := "application/vnd.example.v2+json, application/json;q=0.5"
		ver := VersionFromVendorMediaType(accept)
		if ver != "v2" {
			t.Errorf("expected version v2, got %s", ver)
		}
	})
}
