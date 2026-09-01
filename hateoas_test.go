package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHATEOASLinksAndHAL(t *testing.T) {
	t.Parallel()

	baseURL := "https://api.example.com"

	t.Run("Pending order includes pay, cancel, update, and self links", func(t *testing.T) {
		order := Order{ID: "ord_100", Status: "pending", Amount: 50.0}
		links := OrderStateLinks(baseURL, order)

		if _, ok := links["self"]; !ok {
			t.Error("expected 'self' link")
		}
		if pay, ok := links["pay"]; !ok || pay.Method != http.MethodPost {
			t.Errorf("expected 'pay' POST link, got %v", pay)
		}
		if cancel, ok := links["cancel"]; !ok || cancel.Method != http.MethodPost {
			t.Errorf("expected 'cancel' POST link, got %v", cancel)
		}
		if update, ok := links["update"]; !ok || update.Method != http.MethodPatch {
			t.Errorf("expected 'update' PATCH link, got %v", update)
		}
	})

	t.Run("Paid order includes ship and refund links", func(t *testing.T) {
		order := Order{ID: "ord_200", Status: "paid", Amount: 50.0}
		links := OrderStateLinks(baseURL, order)

		if _, ok := links["pay"]; ok {
			t.Error("paid order should NOT include 'pay' link")
		}
		if ship, ok := links["ship"]; !ok || ship.Method != http.MethodPost {
			t.Errorf("expected 'ship' link, got %v", ship)
		}
		if refund, ok := links["refund"]; !ok || refund.Method != http.MethodPost {
			t.Errorf("expected 'refund' link, got %v", refund)
		}
	})

	t.Run("Shipped order includes track link", func(t *testing.T) {
		order := Order{ID: "ord_300", Status: "shipped", Amount: 50.0}
		links := OrderStateLinks(baseURL, order)

		if track, ok := links["track"]; !ok || track.Method != http.MethodGet {
			t.Errorf("expected 'track' link, got %v", track)
		}
	})

	t.Run("Cancelled and refunded orders have only self link", func(t *testing.T) {
		order := Order{ID: "ord_400", Status: "cancelled", Amount: 50.0}
		links := OrderStateLinks(baseURL, order)

		if len(links) != 1 {
			t.Errorf("expected only 1 link for cancelled order, got %d", len(links))
		}
	})

	t.Run("WriteHalJSON sets application/hal+json content type", func(t *testing.T) {
		order := Order{ID: "ord_500", Status: "pending", Amount: 75.0}
		halRes := WrapOrderWithHal(baseURL, order)

		w := httptest.NewRecorder()
		WriteHalJSON(w, http.StatusOK, halRes)

		if !strings.Contains(w.Header().Get("Content-Type"), "application/hal+json") {
			t.Errorf("expected application/hal+json Content-Type, got %s", w.Header().Get("Content-Type"))
		}

		var parsed HalResource
		if err := json.NewDecoder(w.Body).Decode(&parsed); err != nil {
			t.Fatalf("failed to decode HAL JSON: %v", err)
		}

		if len(parsed.Links) == 0 {
			t.Error("expected parsed _links in HAL response")
		}
	})

	t.Run("BuildCollectionLinks constructs pagination navigation", func(t *testing.T) {
		links := BuildCollectionLinks("https://api.example.com", "/orders", "cur_current", "cur_next", "cur_prev", 20)

		if _, ok := links["self"]; !ok {
			t.Error("expected 'self' link in collection links")
		}
		if next, ok := links["next"]; !ok || !strings.Contains(next.Href, "cursor=cur_next") {
			t.Errorf("expected 'next' link with cur_next, got %v", next)
		}
		if prev, ok := links["prev"]; !ok || !strings.Contains(prev.Href, "cursor=cur_prev") {
			t.Errorf("expected 'prev' link with cur_prev, got %v", prev)
		}
	})
}
