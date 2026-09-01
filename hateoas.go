package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Link represents an RFC 8288 / HAL (Hypertext Application Language) web link.
type Link struct {
	Href      string `json:"href" xml:"href,attr"`
	Method    string `json:"method,omitempty" xml:"method,attr,omitempty"`
	Title     string `json:"title,omitempty" xml:"title,attr,omitempty"`
	Templated bool   `json:"templated,omitempty" xml:"templated,attr,omitempty"`
	Type      string `json:"type,omitempty" xml:"type,attr,omitempty"`
}

// Links is a map of link relations (rel) to Link objects.
type Links map[string]Link

// HalResource wraps any domain entity with HATEOAS hypermedia links (_links).
type HalResource struct {
	Data  any   `json:"data"`
	Links Links `json:"_links"`
}

// HalCollection wraps a list of items with collection-level hypermedia links and metadata.
type HalCollection struct {
	Embedded []any          `json:"_embedded"`
	Links    Links          `json:"_links"`
	Meta     map[string]any `json:"meta,omitempty"`
}

// OrderStateLinks generates available state-transition links based on the current order status.
// This implements Richardson Maturity Model Level 3 (HATEOAS).
func OrderStateLinks(baseURL string, order Order) Links {
	orderURI := fmt.Sprintf("%s/orders/%s", baseURL, order.ID)
	links := Links{
		"self": Link{Href: orderURI, Method: http.MethodGet, Title: "Get order details"},
	}

	switch order.Status {
	case "pending":
		links["pay"] = Link{
			Href:   fmt.Sprintf("%s/orders/%s/pay", baseURL, order.ID),
			Method: http.MethodPost,
			Title:  "Pay for this order",
		}
		links["cancel"] = Link{
			Href:   fmt.Sprintf("%s/orders/%s/cancel", baseURL, order.ID),
			Method: http.MethodPost,
			Title:  "Cancel pending order",
		}
		links["update"] = Link{
			Href:   orderURI,
			Method: http.MethodPatch,
			Title:  "Update order items or details",
		}

	case "paid":
		links["ship"] = Link{
			Href:   fmt.Sprintf("%s/orders/%s/ship", baseURL, order.ID),
			Method: http.MethodPost,
			Title:  "Mark order as shipped",
		}
		links["refund"] = Link{
			Href:   fmt.Sprintf("%s/orders/%s/refund", baseURL, order.ID),
			Method: http.MethodPost,
			Title:  "Request order refund",
		}

	case "shipped":
		links["track"] = Link{
			Href:   fmt.Sprintf("%s/orders/%s/tracking", baseURL, order.ID),
			Method: http.MethodGet,
			Title:  "Track order delivery",
		}

	case "cancelled", "refunded":
		// Terminal states: only self link remains active
	}

	return links
}

// WrapOrderWithHal creates a HalResource for an Order with its dynamic links.
func WrapOrderWithHal(baseURL string, order Order) HalResource {
	return HalResource{
		Data:  order,
		Links: OrderStateLinks(baseURL, order),
	}
}

// BuildCollectionLinks constructs pagination and navigation links for a list of resources.
func BuildCollectionLinks(baseURL, path string, currentCursor, nextCursor, prevCursor string, limit int) Links {
	links := Links{
		"self": Link{Href: fmt.Sprintf("%s%s?limit=%d&cursor=%s", baseURL, path, limit, currentCursor), Method: http.MethodGet},
	}

	if nextCursor != "" {
		links["next"] = Link{Href: fmt.Sprintf("%s%s?limit=%d&cursor=%s", baseURL, path, limit, nextCursor), Method: http.MethodGet, Title: "Next page of results"}
	}
	if prevCursor != "" {
		links["prev"] = Link{Href: fmt.Sprintf("%s%s?limit=%d&cursor=%s", baseURL, path, limit, prevCursor), Method: http.MethodGet, Title: "Previous page of results"}
	}

	return links
}

// WriteHalJSON writes a HAL-compliant response with application/hal+json content type.
func WriteHalJSON(w http.ResponseWriter, status int, resource any) {
	w.Header().Set("Content-Type", "application/hal+json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resource)
}
