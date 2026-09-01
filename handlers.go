package main

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

// Order is our domain model representing a customer order.
type Order struct {
	XMLName   xml.Name  `json:"-" xml:"Order"`
	ID        string    `json:"id" xml:"id"`
	Status    string    `json:"status" xml:"status"`
	Amount    float64   `json:"amount" xml:"amount"`
	Currency  string    `json:"currency" xml:"currency"`
	Version   int       `json:"version" xml:"version"`
	CreatedAt string    `json:"createdAt" xml:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" xml:"updatedAt"`
}

// ETag calculates the entity tag for the order based on ID and version/content.
func (o *Order) ETag() string {
	raw := fmt.Sprintf("%s:%s:%d:%.2f:%s", o.ID, o.Status, o.Version, o.Amount, o.Currency)
	return ComputeETag([]byte(raw))
}

// OrderHandler implements the standard controller pattern with injected dependencies.
type OrderHandler struct {
	mu      sync.RWMutex
	orders  map[string]*Order
	breaker *CircuitBreaker
}

// NewOrderHandler creates a new order handler initialized with seed data.
func NewOrderHandler(breaker *CircuitBreaker) *OrderHandler {
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)

	h := &OrderHandler{
		orders:  make(map[string]*Order),
		breaker: breaker,
	}

	// Seed orders
	h.orders["ord_1"] = &Order{
		ID:        "ord_1",
		Status:    "pending",
		Amount:    149.99,
		Currency:  "USD",
		Version:   1,
		CreatedAt: nowStr,
		UpdatedAt: now,
	}
	h.orders["ord_2"] = &Order{
		ID:        "ord_2",
		Status:    "paid",
		Amount:    299.50,
		Currency:  "USD",
		Version:   1,
		CreatedAt: nowStr,
		UpdatedAt: now,
	}

	return h
}

// List handles GET /orders — demonstrates cursor pagination, multi-field sorting, filtering, and content negotiation.
func (h *OrderHandler) List(w http.ResponseWriter, r *http.Request) {
	params, err := ParsePaginationParams(r, 20, 100, []string{"createdAt", "amount", "status", "id"})
	if err != nil {
		BadRequest(w, r, err.Error())
		return
	}

	statusFilter := r.URL.Query().Get("status")

	h.mu.RLock()
	var filtered []Order
	for _, o := range h.orders {
		if statusFilter == "" || strings.EqualFold(o.Status, statusFilter) {
			filtered = append(filtered, *o)
		}
	}
	h.mu.RUnlock()

	// Check if client specifically requested HAL JSON
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/hal+json") {
		var embedded []any
		for _, o := range filtered {
			embedded = append(embedded, WrapOrderWithHal("", o))
		}
		links := BuildCollectionLinks("", "/orders", params.Cursor, "", "", params.Limit)
		WriteHalJSON(w, http.StatusOK, HalCollection{
			Embedded: embedded,
			Links:    links,
			Meta: map[string]any{
				"requestId": GetRequestID(r.Context()),
				"count":     len(filtered),
			},
		})
		return
	}

	// Apply sparse fieldsets if ?fields= parameter was provided
	if len(params.Fields) > 0 {
		var sparseData []any
		for _, o := range filtered {
			sparseData = append(sparseData, ApplySparseFieldset(o, params.Fields))
		}
		resp := map[string]any{
			"data": sparseData,
			"meta": map[string]any{
				"requestId":    GetRequestID(r.Context()),
				"limit":        params.Limit,
				"appliedSort":  params.Sorts,
				"cursor":       params.Cursor,
				"statusFilter": statusFilter,
			},
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Content negotiation for JSON / XML / CSV
	if strings.Contains(accept, "text/csv") || strings.Contains(accept, "application/xml") {
		RenderResponse(w, r, http.StatusOK, filtered)
		return
	}

	resp := map[string]any{
		"data": filtered,
		"meta": map[string]any{
			"requestId":    GetRequestID(r.Context()),
			"limit":        params.Limit,
			"appliedSort":  params.Sorts,
			"cursor":       params.Cursor,
			"statusFilter": statusFilter,
			"hasMore":      false,
		},
	}
	writeJSON(w, http.StatusOK, resp)
}

// Get handles GET /orders/{id} — demonstrates conditional GET (304 Not Modified), ETags, and Cache-Control.
func (h *OrderHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		BadRequest(w, r, "The order ID is required.")
		return
	}

	h.mu.RLock()
	order, found := h.orders[id]
	if !found {
		h.mu.RUnlock()
		NotFound(w, r, fmt.Sprintf("No order found with ID '%s'.", id))
		return
	}
	orderCopy := *order
	h.mu.RUnlock()

	etag := orderCopy.ETag()

	// Conditional GET: check If-None-Match header
	if CheckIfNoneMatch(r, etag) {
		w.Header().Set("ETag", etag)
		w.WriteHeader(http.StatusNotModified)
		return
	}

	// Set HTTP Caching headers
	SetCacheHeaders(w, etag, orderCopy.UpdatedAt, CacheDirective{
		MaxAge:         30 * time.Second,
		MustRevalidate: true,
		Private:        true,
	})

	// Check if HATEOAS HAL representation requested
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/hal+json") {
		WriteHalJSON(w, http.StatusOK, WrapOrderWithHal("", orderCopy))
		return
	}

	// Sparse fieldset support on single resource
	if fieldsParam := r.URL.Query().Get("fields"); fieldsParam != "" {
		fields := strings.Split(fieldsParam, ",")
		sparseItem := ApplySparseFieldset(orderCopy, fields)
		writeJSON(w, http.StatusOK, sparseItem)
		return
	}

	RenderResponse(w, r, http.StatusOK, orderCopy)
}

type updateOrderRequest struct {
	Amount   *float64 `json:"amount,omitempty"`
	Currency *string  `json:"currency,omitempty"`
}

// Update handles PUT / PATCH /orders/{id} — demonstrates Optimistic Concurrency Control (If-Match -> 412).
func (h *OrderHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.Lock()
	order, found := h.orders[id]
	if !found {
		h.mu.Unlock()
		NotFound(w, r, fmt.Sprintf("No order found with ID '%s'.", id))
		return
	}

	currentETag := order.ETag()

	// Optimistic Concurrency Control: verify If-Match header
	if !CheckIfMatch(r, currentETag) {
		h.mu.Unlock()
		PreconditionFailed(w, r, "The resource has been modified by another client. Please fetch the latest state and retry.")
		return
	}

	var req updateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.mu.Unlock()
		BadRequest(w, r, "Invalid request body JSON.")
		return
	}

	if req.Amount != nil {
		if *req.Amount <= 0 {
			h.mu.Unlock()
			UnprocessableEntity(w, r, "Amount must be positive.", FieldError{Field: "amount", Code: "MUST_BE_POSITIVE"})
			return
		}
		order.Amount = *req.Amount
	}
	if req.Currency != nil {
		order.Currency = *req.Currency
	}

	order.Version++
	order.UpdatedAt = time.Now().UTC()
	updatedOrder := *order
	h.mu.Unlock()

	newETag := updatedOrder.ETag()
	w.Header().Set("ETag", newETag)
	writeJSON(w, http.StatusOK, updatedOrder)
}

// Pay handles POST /orders/{id}/pay — demonstrates state transition endpoint.
func (h *OrderHandler) Pay(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.Lock()
	order, found := h.orders[id]
	if !found {
		h.mu.Unlock()
		NotFound(w, r, fmt.Sprintf("No order found with ID '%s'.", id))
		return
	}

	if order.Status != "pending" {
		h.mu.Unlock()
		Conflict(w, r, fmt.Sprintf("Order '%s' cannot be paid because it is in '%s' status.", id, order.Status))
		return
	}

	order.Status = "paid"
	order.Version++
	order.UpdatedAt = time.Now().UTC()
	updated := *order
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, updated)
}

// Cancel handles POST /orders/{id}/cancel — demonstrates state transition to cancelled.
func (h *OrderHandler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	h.mu.Lock()
	order, found := h.orders[id]
	if !found {
		h.mu.Unlock()
		NotFound(w, r, fmt.Sprintf("No order found with ID '%s'.", id))
		return
	}

	if order.Status == "shipped" || order.Status == "cancelled" {
		h.mu.Unlock()
		Conflict(w, r, fmt.Sprintf("Order '%s' cannot be cancelled because it is already '%s'.", id, order.Status))
		return
	}

	order.Status = "cancelled"
	order.Version++
	order.UpdatedAt = time.Now().UTC()
	updated := *order
	h.mu.Unlock()

	writeJSON(w, http.StatusOK, updated)
}

type createOrderRequest struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency"`
}

// Create handles POST /orders — demonstrates validation, resilience, idempotency, and 201 Created response.
func (h *OrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, r, "The request body is not valid JSON.")
		return
	}

	var fieldErrors []FieldError
	if req.Amount <= 0 {
		fieldErrors = append(fieldErrors, FieldError{Field: "amount", Code: "MUST_BE_POSITIVE"})
	}
	if req.Currency == "" {
		fieldErrors = append(fieldErrors, FieldError{Field: "currency", Code: "REQUIRED"})
	}
	if len(fieldErrors) > 0 {
		UnprocessableEntity(w, r, "Validation error.", fieldErrors...)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	err := h.breaker.Execute(func() error {
		return RetryWithBackoff(ctx, 3, 100*time.Millisecond, func() error {
			return callPaymentService(ctx, req.Amount, req.Currency)
		})
	})
	if err != nil {
		if err == ErrCircuitOpen {
			ServiceUnavailable(w, r, "The payment service is temporarily unavailable, please try again later.", 30)
			return
		}
		InternalError(w, r)
		return
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("ord_%d", now.UnixNano())
	order := &Order{
		ID:        id,
		Status:    "pending",
		Amount:    req.Amount,
		Currency:  req.Currency,
		Version:   1,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now,
	}

	h.mu.Lock()
	h.orders[id] = order
	orderCopy := *order
	h.mu.Unlock()

	w.Header().Set("Location", "/orders/"+order.ID)
	w.Header().Set("ETag", orderCopy.ETag())
	writeJSON(w, http.StatusCreated, orderCopy)
}

// CreateFromForm handles POST /orders/form — demonstrates parsing form URL-encoded body.
func (h *OrderHandler) CreateFromForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		BadRequest(w, r, "The form data could not be parsed.")
		return
	}

	amountStr := r.FormValue("amount")
	currency := r.FormValue("currency")

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		UnprocessableEntity(w, r, "Validation error.",
			FieldError{Field: "amount", Code: "INVALID_OR_MISSING"})
		return
	}
	if currency == "" {
		UnprocessableEntity(w, r, "Validation error.",
			FieldError{Field: "currency", Code: "REQUIRED"})
		return
	}

	now := time.Now().UTC()
	id := fmt.Sprintf("ord_form_%d", now.UnixNano())
	order := &Order{
		ID:        id,
		Status:    "pending",
		Amount:    amount,
		Currency:  currency,
		Version:   1,
		CreatedAt: now.Format(time.RFC3339),
		UpdatedAt: now,
	}

	h.mu.Lock()
	h.orders[id] = order
	orderCopy := *order
	h.mu.Unlock()

	w.Header().Set("Location", "/orders/"+order.ID)
	w.Header().Set("ETag", orderCopy.ETag())
	writeJSON(w, http.StatusCreated, orderCopy)
}

// Events handles GET /orders/{id}/events — demonstrates Server-Sent Events (SSE) streaming.
func (h *OrderHandler) Events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		InternalError(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case t := <-ticker.C:
			event := map[string]string{
				"event":     "order.status.updated",
				"orderId":   chi.URLParam(r, "id"),
				"timestamp": t.Format(time.RFC3339),
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// callPaymentService simulates a downstream call.
func callPaymentService(ctx context.Context, amount float64, currency string) error {
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
