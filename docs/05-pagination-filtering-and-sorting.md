# Pattern 05: Pagination, Filtering, Sorting & Sparse Fieldsets

## 1. Executive Summary & Production Comparison

Large collections must never be returned in an unbounded single response. Unbounded collection endpoints cause high database CPU, memory exhaustion (OOM crashes), network congestion, and degraded client rendering performance.

There are three primary pagination paradigms:

| Pagination Strategy | Query Signature | Advantages | Disadvantages | Best For |
|---|---|---|---|---|
| **Offset / Limit** | `?offset=200&limit=20` | Simple to implement, allows jumping directly to page N. | $O(N)$ database scan performance degrade (`OFFSET 1000000`), suffers from the **Page Drift / Missing Item Problem** when records are inserted/deleted during pagination. | Small static datasets, internal admin tables. |
| **Keyset (Seek)** | `?since_id=ord_99&limit=20` | High performance (uses index seek $O(\log N)$), no page drift. | Cannot jump to arbitrary page N, tight coupling to single sequential column (ID). | Public APIs, sequential streaming. |
| **Opaque Cursor** | `?cursor=eyJpZCI6...&limit=20` | High performance, fully decouples internal storage representation from public API contract, supports bi-directional pagination (`next`/`prev`). | Cursors are opaque tokens that cannot be manually constructed by clients. | **Gold standard for enterprise APIs (Stripe, GitHub, Slack).** |

---

## 2. The Page Drift & Data Skipping Problem

When using classic offset pagination, concurrent insertions or deletions cause clients to either see duplicate items or skip records completely:

```
Initial State: [A, B, C, D, E, F]
Page 1 (offset=0, limit=3): Returns [A, B, C]

Concurrent Action: Record 'NEW' is inserted at the top -> [NEW, A, B, C, D, E, F]
Page 2 (offset=3, limit=3): Returns [C, D, E]   <-- Record 'C' was returned twice!
```

**Cursor-based pagination eliminates this entirely:** The cursor encodes a pointer to the specific boundary record (`ID` + timestamp) rather than a dynamic offset index.

---

## 3. Opaque Cursor Implementation in Go

```go
type CursorPayload struct {
	ID        string `json:"id"`
	Timestamp int64  `json:"ts"`
	Direction string `json:"dir"` // "next" or "prev"
}

func EncodeCursor(id string, createdAt time.Time, direction string) string {
	payload := CursorPayload{
		ID:        id,
		Timestamp: createdAt.UnixNano(),
		Direction: direction,
	}
	bytes, _ := json.Marshal(payload)
	return base64.URLEncoding.EncodeToString(bytes)
}

func DecodeCursor(token string) (*CursorPayload, error) {
	if token == "" {
		return nil, nil
	}
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return nil, errors.New("invalid cursor: malformed base64")
	}
	var payload CursorPayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, errors.New("invalid cursor: corrupt JSON")
	}
	return &payload, nil
}
```

---

## 4. Multi-Field Sorting Syntax

Industry-standard REST APIs accept sorting directives via the `sort` query parameter:
- Prefix `-` denotes descending order (`-createdAt`).
- Prefix `+` or no prefix denotes ascending order (`amount` or `+amount`).
- Multiple sort keys are comma-separated: `?sort=-createdAt,amount`.

```go
func ParseSortParam(sortStr string, allowedFields []string) ([]SortField, error) {
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
		sorts = append(sorts, SortField{Field: fieldName, Order: order})
	}
	return sorts, nil
}
```

---

## 5. Sparse Fieldsets (Field Filtering)

Sparse fieldsets allow clients to request only a subset of fields from a resource, significantly reducing payload size and network bandwidth:

```http
GET /orders?fields=id,status,amount
```

### Go Implementation with Reflection

```go
func ApplySparseFieldset(item any, requestedFields []string) any {
	if len(requestedFields) == 0 {
		return item
	}
	fieldSet := make(map[string]bool)
	for _, f := range requestedFields {
		fieldSet[strings.ToLower(f)] = true
	}

	val := reflect.ValueOf(item)
	if val.Kind() == reflect.Struct {
		filtered := make(map[string]any)
		typ := val.Type()
		for i := range val.NumField() {
			field := typ.Field(i)
			tagKey := strings.Split(field.Tag.Get("json"), ",")[0]
			if tagKey == "" || tagKey == "-" {
				tagKey = field.Name
			}
			if fieldSet[strings.ToLower(tagKey)] {
				filtered[tagKey] = val.Field(i).Interface()
			}
		}
		return filtered
	}
	return item
}
```
