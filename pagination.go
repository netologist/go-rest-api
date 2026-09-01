package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// CursorPayload represents internal pagination state serialized into an opaque client token.
type CursorPayload struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"ts"`
	Direction string `json:"dir"` // "next" or "prev"
}

// EncodeCursor converts the cursor payload into an opaque base64 string.
func EncodeCursor(id string, createdAt time.Time, direction string) string {
	payload := CursorPayload{
		ID:        id,
		Timestamp: createdAt.UnixNano(),
		Direction: direction,
	}
	bytes, _ := json.Marshal(payload)
	return base64.URLEncoding.EncodeToString(bytes)
}

// DecodeCursor parses and validates an inbound opaque cursor string.
func DecodeCursor(token string) (*CursorPayload, error) {
	if token == "" {
		return nil, nil
	}

	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("invalid cursor format: not valid base64")
	}

	var payload CursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, errors.New("invalid cursor payload: corrupt JSON")
	}

	return &payload, nil
}

// SortOrder defines sort direction.
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// SortField represents a single sort directive in multi-field sorting.
type SortField struct {
	Field string    `json:"field"`
	Order SortOrder `json:"order"`
}

// ParseSortParam parses comma-separated sort strings like "-createdAt,amount,+status".
func ParseSortParam(sortStr string, allowedFields []string) ([]SortField, error) {
	if strings.TrimSpace(sortStr) == "" {
		return nil, nil
	}

	allowedMap := make(map[string]bool)
	for _, f := range allowedFields {
		allowedMap[strings.ToLower(f)] = true
	}

	var sorts []SortField
	tokens := strings.Split(sortStr, ",")

	for _, token := range tokens {
		t := strings.TrimSpace(token)
		if t == "" {
			continue
		}

		order := SortAsc
		fieldName := t

		if strings.HasPrefix(t, "-") {
			order = SortDesc
			fieldName = strings.TrimPrefix(t, "-")
		} else if strings.HasPrefix(t, "+") {
			order = SortAsc
			fieldName = strings.TrimPrefix(t, "+")
		}

		if len(allowedMap) > 0 && !allowedMap[strings.ToLower(fieldName)] {
			return nil, fmt.Errorf("invalid sort field '%s'; allowed fields are: %v", fieldName, allowedFields)
		}

		sorts = append(sorts, SortField{Field: fieldName, Order: order})
	}

	return sorts, nil
}

// PaginationParams captures all standard collection query parameters.
type PaginationParams struct {
	Limit      int         `json:"limit"`
	Cursor     string      `json:"cursor"`
	Sorts      []SortField `json:"sorts"`
	Fields     []string    `json:"fields"`
	DecodedCur *CursorPayload
}

// ParsePaginationParams extracts and validates pagination, sorting, and field-filtering params from request.
func ParsePaginationParams(r *http.Request, defaultLimit, maxLimit int, allowedSortFields []string) (*PaginationParams, error) {
	q := r.URL.Query()

	limit := defaultLimit
	if lStr := q.Get("limit"); lStr != "" {
		parsed, err := strconv.Atoi(lStr)
		if err != nil || parsed <= 0 || parsed > maxLimit {
			return nil, fmt.Errorf("the 'limit' parameter must be between 1 and %d", maxLimit)
		}
		limit = parsed
	}

	cursorStr := q.Get("cursor")
	decodedCur, err := DecodeCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	sorts, err := ParseSortParam(q.Get("sort"), allowedSortFields)
	if err != nil {
		return nil, err
	}

	var fields []string
	if fieldsStr := q.Get("fields"); fieldsStr != "" {
		for _, f := range strings.Split(fieldsStr, ",") {
			if trimmed := strings.TrimSpace(f); trimmed != "" {
				fields = append(fields, trimmed)
			}
		}
	}

	return &PaginationParams{
		Limit:      limit,
		Cursor:     cursorStr,
		Sorts:      sorts,
		Fields:     fields,
		DecodedCur: decodedCur,
	}, nil
}

// ApplySparseFieldset filters a struct or map down to only requested field keys.
// If requestedFields is empty, the original item is returned unmodified.
func ApplySparseFieldset(item any, requestedFields []string) any {
	if len(requestedFields) == 0 {
		return item
	}

	fieldSet := make(map[string]bool)
	for _, f := range requestedFields {
		fieldSet[strings.ToLower(f)] = true
	}

	val := reflect.ValueOf(item)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() == reflect.Map {
		filtered := make(map[string]any)
		iter := val.MapRange()
		for iter.Next() {
			k := fmt.Sprintf("%v", iter.Key().Interface())
			if fieldSet[strings.ToLower(k)] {
				filtered[k] = iter.Value().Interface()
			}
		}
		return filtered
	}

	if val.Kind() == reflect.Struct {
		filtered := make(map[string]any)
		typ := val.Type()
		for i := range val.NumField() {
			field := typ.Field(i)
			jsonTag := field.Tag.Get("json")
			tagKey := strings.Split(jsonTag, ",")[0]
			if tagKey == "" || tagKey == "-" {
				tagKey = field.Name
			}

			if fieldSet[strings.ToLower(tagKey)] || fieldSet[strings.ToLower(field.Name)] {
				filtered[tagKey] = val.Field(i).Interface()
			}
		}
		return filtered
	}

	return item
}
