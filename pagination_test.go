package main

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

func TestCursorPaginationAndSort(t *testing.T) {
	t.Parallel()

	t.Run("Encode and Decode cursor roundtrip", func(t *testing.T) {
		now := time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
		id := "ord_cursor_10"
		direction := "next"

		token := EncodeCursor(id, now, direction)
		if token == "" {
			t.Fatal("expected non-empty cursor string")
		}

		decoded, err := DecodeCursor(token)
		if err != nil {
			t.Fatalf("failed to decode cursor: %v", err)
		}

		if decoded.ID != id {
			t.Errorf("expected decoded ID %s, got %s", id, decoded.ID)
		}
		if decoded.Direction != direction {
			t.Errorf("expected direction %s, got %s", direction, decoded.Direction)
		}
		if decoded.Timestamp != now.UnixNano() {
			t.Errorf("expected timestamp %d, got %d", now.UnixNano(), decoded.Timestamp)
		}
	})

	t.Run("Invalid base64 cursor returns error", func(t *testing.T) {
		_, err := DecodeCursor("!not-valid-base64!")
		if err == nil {
			t.Fatal("expected error on invalid base64, got nil")
		}
	})

	t.Run("ParseSortParam handles multiple fields with asc/desc prefixes", func(t *testing.T) {
		sortStr := "-createdAt,amount,+status"
		allowed := []string{"createdAt", "amount", "status", "id"}

		sorts, err := ParseSortParam(sortStr, allowed)
		if err != nil {
			t.Fatalf("failed to parse sort param: %v", err)
		}

		if len(sorts) != 3 {
			t.Fatalf("expected 3 sort directives, got %d", len(sorts))
		}
		if sorts[0].Field != "createdAt" || sorts[0].Order != SortDesc {
			t.Errorf("expected -createdAt to be desc, got %v", sorts[0])
		}
		if sorts[1].Field != "amount" || sorts[1].Order != SortAsc {
			t.Errorf("expected amount to be asc, got %v", sorts[1])
		}
		if sorts[2].Field != "status" || sorts[2].Order != SortAsc {
			t.Errorf("expected +status to be asc, got %v", sorts[2])
		}
	})

	t.Run("Disallowed sort field triggers error", func(t *testing.T) {
		_, err := ParseSortParam("unsupported_field", []string{"id", "createdAt"})
		if err == nil {
			t.Fatal("expected error for unsupported sort field, got nil")
		}
	})

	t.Run("ParsePaginationParams correctly parses query strings", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/orders?limit=50&sort=-createdAt&fields=id,status", nil)
		params, err := ParsePaginationParams(req, 20, 100, []string{"createdAt", "amount", "id"})
		if err != nil {
			t.Fatalf("unexpected error parsing params: %v", err)
		}

		if params.Limit != 50 {
			t.Errorf("expected limit 50, got %d", params.Limit)
		}
		if len(params.Fields) != 2 || params.Fields[0] != "id" || params.Fields[1] != "status" {
			t.Errorf("expected fields [id, status], got %v", params.Fields)
		}
	})
}

func TestApplySparseFieldset(t *testing.T) {
	t.Parallel()

	order := Order{
		ID:        "ord_sparse_1",
		Status:    "paid",
		Amount:    199.99,
		Currency:  "USD",
		CreatedAt: "2026-08-29T12:00:00Z",
	}

	t.Run("Filters struct to only requested fields", func(t *testing.T) {
		fields := []string{"id", "amount"}
		filtered := ApplySparseFieldset(order, fields)

		m, ok := filtered.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any result, got %T", filtered)
		}

		if len(m) != 2 {
			t.Errorf("expected 2 keys, got %d: %v", len(m), m)
		}
		if m["id"] != "ord_sparse_1" {
			t.Errorf("expected id 'ord_sparse_1', got %v", m["id"])
		}
		if m["amount"] != 199.99 {
			t.Errorf("expected amount 199.99, got %v", m["amount"])
		}
		if _, exists := m["status"]; exists {
			t.Error("status field should have been filtered out")
		}
	})

	t.Run("Empty fields slice returns original unmodified item", func(t *testing.T) {
		unmodified := ApplySparseFieldset(order, nil)
		if !reflect.DeepEqual(unmodified, order) {
			t.Errorf("expected unmodified struct, got %v", unmodified)
		}
	})
}
