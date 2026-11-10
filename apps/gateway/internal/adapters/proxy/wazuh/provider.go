package wazuh

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Provider struct{ client *Client }

var _ capability.EventSource = (*Provider)(nil)
var _ capability.EventAggregator = (*Provider)(nil)
var _ capability.SourceProber = (*Provider)(nil)
var _ capability.EventCursorCloser = (*Provider)(nil)

func NewProvider(cfg ClientConfig) (registry.Provider, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return registry.Provider{}, err
	}
	p := &Provider{client: client}
	return registry.Provider{Source: domain.Source{Code: SourceCode, Name: "Wazuh", Kind: "siem", Mode: "proxy", Status: "offline", Capabilities: []domain.Capability{domain.CapabilityEvents}}, CredentialSecret: CredentialSelector, Events: p, EventAggregation: p, Prober: p}, nil
}

func (provider *Provider) SearchEvents(ctx context.Context, access capability.Access, request capability.SearchEventsRequest) (page capability.EventPage, err error) {
	if request.Limit < 1 || request.Limit > 100 {
		return page, sourceRequestError("invalid_limit", "limit must be between 1 and 100")
	}
	query, err := provider.eventQuery(request)
	if err != nil {
		return page, translateError(err)
	}
	sort, err := provider.client.buildSort(request.Sort)
	if err != nil {
		return page, err
	}
	includes := append([]string(nil), sourceIncludes...)
	for _, field := range request.Columns {
		spec, ok := lookupField(field)
		if !ok {
			return page, sourceRequestError("invalid_source_request", "columns contains an unsupported field")
		}
		includes = append(includes, spec.Path)
	}
	cursor := providerCursor{Version: 1}
	if request.Cursor != "" {
		cursor, err = decodeProviderCursor(request.Cursor)
		if err != nil {
			return page, err
		}
		if len(cursor.Sort) != 0 && len(cursor.Sort) != len(sort) {
			return page, sourceRequestError("invalid_cursor", "cursor sort is invalid")
		}
	} else {
		cursor.PIT, err = provider.client.openPIT(ctx, access)
		if err != nil {
			return page, err
		}
		defer func() {
			if err != nil {
				provider.release(ctx, access, cursor.PIT)
			}
		}()
	}
	// A page may be replayed by the merger even when it is the final vendor page.
	page.ReplayCursor, err = encodeProviderCursor(cursor)
	if err != nil {
		return page, err
	}
	response, err := provider.client.search(ctx, access, searchRequest{Size: request.Limit + 1, TrackTotalHits: cursor.Total == nil, PIT: &pointInTime{ID: cursor.PIT, KeepAlive: pitKeepAlive}, Query: query, Sort: sort, SearchAfter: cursor.Sort, Source: sourceFilter{Includes: includes}})
	if err != nil {
		return page, err
	}
	if response.TimedOut || response.Shards.Failed > 0 {
		return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh search is incomplete"}
	}
	if len(response.Hits.Hits) > request.Limit+1 {
		return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh response exceeds page size"}
	}
	if cursor.Total == nil && response.Hits.Total.Relation == "eq" {
		total := response.Hits.Total.Value
		if total < 0 {
			return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh total is invalid"}
		}
		cursor.Total = &total
	}
	page.Total = cursor.Total
	page.Status = "complete"
	more := len(response.Hits.Hits) > request.Limit
	hits := response.Hits.Hits
	if more {
		hits = hits[:request.Limit]
	}
	for _, hit := range hits {
		if err = provider.client.validateIndex(hit.Index); err != nil {
			return page, err
		}
		if _, _, err = provider.client.validateRef(hit.Index + "/" + hit.ID); err != nil {
			return page, err
		}
		if len(hit.Sort) != len(sort) {
			return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh sort values are invalid"}
		}
		// A configured doc-values key must be an exact copy of the native _id.
		// Missing or different keys cannot silently weaken search_after ordering.
		for i, clause := range sort {
			if _, ok := clause[provider.client.documentIDField]; ok {
				if value, ok := hit.Sort[i].(string); !ok || value != hit.ID {
					return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh document ID sort field is missing or differs from _id"}
				}
			}
		}
		var mapped mappedAlert
		mapped, err = mapHit(hit.Index, hit.ID, hit.Source, provider.client.now().UTC())
		if err != nil {
			return page, translateError(err)
		}
		fields := append([]string(nil), request.Columns...)
		for _, rule := range request.Sort {
			fields = append(fields, rule.Field)
		}
		applyColumns(&mapped.Event, fields, hit.Source)
		page.Events = append(page.Events, mapped.Event)
		page.Entities = append(page.Entities, mapped.Entities...)
		page.Relations = append(page.Relations, mapped.Relations...)
	}
	if more {
		cursor.Sort = hits[len(hits)-1].Sort
		page.NextCursor, err = encodeProviderCursor(cursor)
	}
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	return page, err
}

func applyColumns(event *domain.Event, columns []string, source alertSource) {
	raw, _ := json.Marshal(source)
	values := map[string]any{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	_ = decoder.Decode(&values)
	for _, column := range columns {
		spec, ok := lookupField(column)
		if !ok {
			continue
		}
		var value any = values
		for _, part := range strings.Split(spec.Path, ".") {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[part]
		}
		if bounded, ok := projectValue(value); ok {
			event.Attributes[column] = bounded
		}
	}
}
func projectValue(value any) (any, bool) {
	switch v := value.(type) {
	case string:
		return boundText(v, 1024), true
	case json.Number, bool:
		return v, true
	case []any:
		out := []any{}
		for _, v := range v {
			if len(out) >= 32 {
				break
			}
			if scalar, ok := projectValue(v); ok {
				switch scalar.(type) {
				case string, json.Number, bool:
					out = append(out, scalar)
				}
			}
		}
		return out, true
	default:
		return nil, false
	}
}

func (provider *Provider) CloseEventCursor(ctx context.Context, access capability.Access, raw string) error {
	if raw == "" {
		return nil
	}
	cursor, err := decodeProviderCursor(raw)
	if err != nil {
		return err
	}
	return provider.client.closePIT(ctx, access, cursor.PIT)
}
func (provider *Provider) release(ctx context.Context, access capability.Access, pit string) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	_ = provider.client.closePIT(cleanup, access, pit)
}

func (provider *Provider) Probe(ctx context.Context, access capability.Access) (string, error) {
	response, err := provider.client.search(ctx, access, searchRequest{Size: 0, Source: sourceFilter{Includes: []string{"id"}}, Query: map[string]any{"match_all": map[string]any{}}})
	if err != nil {
		return "offline", err
	}
	if response.TimedOut || response.Shards.Failed > 0 {
		return "degraded", nil
	}
	return "online", nil
}

func (provider *Provider) ResolveContext(ctx context.Context, access capability.Access, request capability.ResolveContextRequest) (capability.ContextPage, error) {
	page := capability.ContextPage{}
	type documentRef struct {
		Index  string   `json:"_index"`
		ID     string   `json:"_id"`
		Source []string `json:"_source"`
	}
	docs := []documentRef{}
	wanted := map[string]bool{}
	for _, id := range request.EventIDs {
		if wanted[id] {
			continue
		}
		index, doc, err := provider.client.validateRef(id)
		if err != nil {
			return page, err
		}
		wanted[id] = true
		docs = append(docs, documentRef{index, doc, sourceIncludes})
	}
	entities := []domain.EntityRef{}
	for _, id := range request.EntityIDs {
		kind, value, ok := strings.Cut(id, ":")
		if !ok || kind == "" || value == "" || value != domain.CanonicalValue(kind, value) {
			return page, sourceRequestError("invalid_source_ref", "source entity ID is invalid")
		}
		if _, err := entityFields(kind, value); err != nil {
			return page, translateError(err)
		}
		entities = append(entities, domain.EntityRef{Type: kind, Value: value})
	}
	var partial error
	for len(docs) > 0 {
		n := len(docs)
		if n > 100 {
			n = 100
		}
		batch := docs[:n]
		docs = docs[n:]
		var response struct {
			Docs []struct {
				Index  string          `json:"_index"`
				ID     string          `json:"_id"`
				Found  bool            `json:"found"`
				Source alertSource     `json:"_source"`
				Error  json.RawMessage `json:"error"`
			} `json:"docs"`
		}
		err := provider.client.doJSON(ctx, access, http.MethodPost, "_mget", nil, struct {
			Docs []documentRef `json:"docs"`
		}{batch}, &response)
		if err != nil {
			return page, err
		}
		if len(response.Docs) != len(batch) {
			return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh document response is invalid"}
		}
		for i, doc := range response.Docs {
			if doc.Index != batch[i].Index || doc.ID != batch[i].ID {
				return page, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh document identity is invalid"}
			}
			if len(doc.Error) > 0 && string(doc.Error) != "null" {
				partial = &domain.UpstreamError{StatusCode: 502, Message: "Wazuh document is unavailable"}
				continue
			}
			if !doc.Found {
				if partial == nil {
					partial = domain.ErrNotFound
				}
				continue
			}
			mapped, err := mapHit(doc.Index, doc.ID, doc.Source, provider.client.now().UTC())
			if err != nil {
				partial = translateError(err)
				continue
			}
			page.Events = append(page.Events, mapped.Event)
			page.Entities = append(page.Entities, mapped.Entities...)
			page.Relations = append(page.Relations, mapped.Relations...)
		}
	}
	for _, entity := range entities {
		query, err := buildQuery(time.Unix(0, 0), provider.client.now().UTC(), []domain.EntityRef{entity}, "", nil, nil)
		if err != nil {
			return page, translateError(err)
		}
		sort, _ := provider.client.buildSort(nil)
		response, err := provider.client.search(ctx, access, searchRequest{Size: 1, Query: query, Sort: sort, Source: sourceFilter{Includes: sourceIncludes}})
		if err != nil {
			return page, err
		}
		found := false
		for _, hit := range response.Hits.Hits {
			if err := provider.client.validateIndex(hit.Index); err != nil {
				return page, err
			}
			mapped, err := mapHit(hit.Index, hit.ID, hit.Source, provider.client.now().UTC())
			if err != nil {
				return page, translateError(err)
			}
			for _, candidate := range mapped.Entities {
				if candidate.Type == entity.Type && candidate.Value == entity.Value {
					page.Entities = append(page.Entities, candidate)
					found = true
				}
			}
		}
		if !found && partial == nil {
			partial = domain.ErrNotFound
		}
	}
	page.Events = normalization.Events(page.Events)
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	return page, partial
}
