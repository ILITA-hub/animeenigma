package pagination

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cursor represents a pagination cursor
type Cursor struct {
	ID        string    `json:"id,omitempty"`
	Timestamp time.Time `json:"ts,omitempty"`
	Offset    int       `json:"off,omitempty"`
	SortValue string    `json:"sv,omitempty"`
}

// Encode encodes the cursor to a base64 string
func (c Cursor) Encode() string {
	data, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(data)
}

// DecodeCursor decodes a base64 string to a Cursor
func DecodeCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}

	data, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("decode cursor: %w", err)
	}

	var cursor Cursor
	if err := json.Unmarshal(data, &cursor); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return &cursor, nil
}

// PageInfo contains pagination information for responses
type PageInfo struct {
	HasNextPage     bool   `json:"has_next_page"`
	HasPreviousPage bool   `json:"has_previous_page"`
	StartCursor     string `json:"start_cursor,omitempty"`
	EndCursor       string `json:"end_cursor,omitempty"`
	TotalCount      *int64 `json:"total_count,omitempty"`
}

// PageRequest represents a pagination request
type PageRequest struct {
	First  *int    // Forward pagination: first N items
	After  string  // Forward pagination: after this cursor
	Last   *int    // Backward pagination: last N items
	Before string  // Backward pagination: before this cursor
}

// Limit returns the limit for the query
func (p PageRequest) Limit() int {
	if p.First != nil && *p.First > 0 {
		return min(*p.First, MaxPageSize)
	}
	if p.Last != nil && *p.Last > 0 {
		return min(*p.Last, MaxPageSize)
	}
	return DefaultPageSize
}

// IsForward returns true if this is forward pagination
func (p PageRequest) IsForward() bool {
	return p.First != nil || p.After != ""
}

// IsBackward returns true if this is backward pagination
func (p PageRequest) IsBackward() bool {
	return p.Last != nil || p.Before != ""
}

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

// OffsetPageRequest represents simple offset-based pagination
type OffsetPageRequest struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// Offset returns the SQL offset
func (p OffsetPageRequest) Offset() int {
	if p.Page < 1 {
		p.Page = 1
	}
	return (p.Page - 1) * p.Limit()
}

// Limit returns the SQL limit
func (p OffsetPageRequest) Limit() int {
	if p.PageSize <= 0 {
		return DefaultPageSize
	}
	return min(p.PageSize, MaxPageSize)
}

// OffsetPageInfo contains offset-based pagination info
type OffsetPageInfo struct {
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	TotalPages int   `json:"total_pages"`
	TotalCount int64 `json:"total_count"`
}

// NewOffsetPageInfo creates pagination info from count and request
func NewOffsetPageInfo(totalCount int64, req OffsetPageRequest) OffsetPageInfo {
	pageSize := req.Limit()
	totalPages := int((totalCount + int64(pageSize) - 1) / int64(pageSize))
	page := req.Page
	if page < 1 {
		page = 1
	}

	return OffsetPageInfo{
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
		TotalCount: totalCount,
	}
}

// Edge wraps an item with its cursor for cursor-based pagination
type Edge[T any] struct {
	Node   T      `json:"node"`
	Cursor string `json:"cursor"`
}

// Connection represents a paginated list with cursor-based pagination
type Connection[T any] struct {
	Edges    []Edge[T] `json:"edges"`
	PageInfo PageInfo  `json:"page_info"`
}

// NewConnection creates a connection from items
func NewConnection[T any](items []T, cursorFn func(T) Cursor, hasMore bool, hasPrevious bool) Connection[T] {
	edges := make([]Edge[T], len(items))
	for i, item := range items {
		edges[i] = Edge[T]{
			Node:   item,
			Cursor: cursorFn(item).Encode(),
		}
	}

	pageInfo := PageInfo{
		HasNextPage:     hasMore,
		HasPreviousPage: hasPrevious,
	}

	if len(edges) > 0 {
		pageInfo.StartCursor = edges[0].Cursor
		pageInfo.EndCursor = edges[len(edges)-1].Cursor
	}

	return Connection[T]{
		Edges:    edges,
		PageInfo: pageInfo,
	}
}

// ParseIntParam parses an integer from a string with a default value
func ParseIntParam(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}

// SortOrder represents sort direction
type SortOrder string

const (
	SortAsc  SortOrder = "asc"
	SortDesc SortOrder = "desc"
)

// ParseSortOrder parses a sort order string
func ParseSortOrder(s string) SortOrder {
	switch strings.ToLower(s) {
	case "asc", "ascending":
		return SortAsc
	case "desc", "descending":
		return SortDesc
	default:
		return SortDesc
	}
}
