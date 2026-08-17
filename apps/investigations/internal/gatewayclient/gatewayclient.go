// Package gatewayclient — вызов IR Gateway из ir-api: attachEvents с query
// резолвит выборку через POST /api/v1/events/search. На демо-стенде gateway
// работает с AUTH_DISABLED, поэтому клиент несёт только X-Project-ID.
package gatewayclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Configured() bool { return c.baseURL != "" }

// UpstreamError — не-2xx ответ gateway с сохранённым телом.
type UpstreamError struct {
	Status int
	Body   string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("gateway search: upstream status %d: %s", e.Status, e.Body)
}

type TimeRange struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// EntityRef — условие поиска по сущности; событие матчится, если содержит
// хотя бы одну из перечисленных сущностей (точное совпадение type+value).
type EntityRef struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type SearchRequest struct {
	Sources   []string    `json:"sources,omitempty"`
	TimeRange *TimeRange  `json:"time_range,omitempty"`
	Query     string      `json:"query,omitempty"`
	Entities  []EntityRef `json:"entities,omitempty"`
	Limit     int         `json:"limit,omitempty"`
	Cursor    string      `json:"cursor,omitempty"`
}

type Provenance struct {
	Source     string  `json:"source"`
	ExternalID string  `json:"external_id"`
	SourceURL  *string `json:"source_url"`
}

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Title      string         `json:"title"`
	Severity   string         `json:"severity"`
	OccurredAt time.Time      `json:"occurred_at"`
	EntityIDs  []string       `json:"entity_ids"`
	Attributes map[string]any `json:"attributes"`
	Provenance Provenance     `json:"provenance"`
}

type SearchResponse struct {
	Events     []Event `json:"events"`
	NextCursor *string `json:"next_cursor"`
}

func (c *Client) SearchEvents(ctx context.Context, projectID string, request SearchRequest) (SearchResponse, error) {
	encoded, err := json.Marshal(request)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("encode search request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/events/search", bytes.NewReader(encoded))
	if err != nil {
		return SearchResponse{}, fmt.Errorf("build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Project-ID", projectID)

	resp, err := c.http.Do(req)
	if err != nil {
		return SearchResponse{}, &UpstreamError{Status: 0, Body: err.Error()}
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return SearchResponse{}, &UpstreamError{Status: resp.StatusCode, Body: "read body: " + err.Error()}
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return SearchResponse{}, &UpstreamError{Status: resp.StatusCode, Body: strings.TrimSpace(string(body))}
	}

	var out SearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return SearchResponse{}, &UpstreamError{Status: resp.StatusCode, Body: "decode response: " + err.Error()}
	}
	return out, nil
}
