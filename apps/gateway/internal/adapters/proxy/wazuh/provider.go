package wazuh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/normalization"
	"github.com/sb0rka/ir/apps/gateway/internal/proxy"
	"github.com/sb0rka/ir/apps/gateway/internal/registry"
)

type Provider struct {
	client *Client
}

var (
	_ capability.EventSource     = (*Provider)(nil)
	_ capability.EventAggregator = (*Provider)(nil)
	_ capability.SourceProber    = (*Provider)(nil)
)

func NewProvider(cfg ClientConfig) (registry.Provider, error) {
	client, err := NewClient(cfg)
	if err != nil {
		return registry.Provider{}, err
	}
	adapter := &Provider{client: client}
	return registry.Provider{
		Source: domain.Source{
			Code:   SourceCode,
			Name:   "Wazuh",
			Kind:   "siem",
			Mode:   "proxy",
			Status: "offline",
			Capabilities: []domain.Capability{
				domain.CapabilityEvents,
			},
		},
		CredentialMode:   registry.CredentialModeProcess,
		Events:           adapter,
		EventAggregation: adapter,
		Prober:           adapter,
	}, nil
}

func NewProviderFromHTTP(httpCfg proxy.HTTPClientConfig, username, password, indexPattern string, skipTLS bool) (registry.Provider, error) {
	httpCfg.SkipTLSVerify = skipTLS
	return NewProvider(ClientConfig{
		HTTP:         httpCfg,
		Username:     username,
		Password:     password,
		IndexPattern: indexPattern,
	})
}

func (provider *Provider) SearchEvents(ctx context.Context, _ capability.Access, request capability.SearchEventsRequest) (capability.EventPage, error) {
	if provider == nil || provider.client == nil {
		return capability.EventPage{}, sourceRequestError("source_unavailable", "Wazuh client is not configured")
	}
	built, err := buildSearchRequest(provider.client, request)
	if err != nil {
		return capability.EventPage{}, translateError(err)
	}
	response, err := provider.client.Search(ctx, built.Body)
	if err != nil {
		return capability.EventPage{}, translateError(err)
	}
	fetchedAt := provider.client.now().UTC()
	page := capability.EventPage{Status: "complete"}
	if response.TimedOut || response.Shards.Failed > 0 {
		page.Status = "truncated"
	}
	if response.Hits.Total.Relation == "eq" {
		total := response.Hits.Total.Value
		page.Total = &total
	}
	for _, hit := range response.Hits.Hits {
		mapped, mapErr := mapHit(hit.Index, hit.ID, hit.Source, fetchedAt)
		if mapErr != nil {
			return capability.EventPage{}, translateError(mapErr)
		}
		applyColumns(&mapped.Event, request.Columns, hit.Source)
		page.Events = append(page.Events, mapped.Event)
		page.Entities = append(page.Entities, mapped.Entities...)
		page.Relations = append(page.Relations, mapped.Relations...)
	}
	page.Events = normalization.Events(page.Events)
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	if len(response.Hits.Hits) >= request.Limit && len(response.Hits.Hits) > 0 {
		last := response.Hits.Hits[len(response.Hits.Hits)-1]
		encoded, encodeErr := encodeCursor(providerCursor{Version: 1, Sort: last.Sort, Fingerprint: built.Fingerprint})
		if encodeErr != nil {
			return capability.EventPage{}, translateError(encodeErr)
		}
		page.NextCursor = encoded
	}
	return page, nil
}

func applyColumns(event *domain.Event, columns []string, source alertSource) {
	if event.Attributes == nil {
		event.Attributes = map[string]any{}
	}
	for _, column := range columns {
		spec, ok := lookupField(column)
		if !ok || spec.Path == "" {
			continue
		}
		if value, ok := columnValue(spec.Path, source); ok {
			event.Attributes[column] = value
		}
	}
}

func columnValue(path string, source alertSource) (any, bool) {
	switch path {
	case "rule.description":
		return boundText(source.Rule.Description, 256), source.Rule.Description != ""
	case "rule.id":
		return source.Rule.ID, source.Rule.ID != ""
	case "rule.level":
		return source.Rule.Level, true
	case "rule.groups":
		return source.Rule.Groups, len(source.Rule.Groups) > 0
	case "agent.name":
		return source.Agent.Name, source.Agent.Name != ""
	case "agent.ip":
		return source.Agent.IP, source.Agent.IP != ""
	case "agent.id":
		return source.Agent.ID, source.Agent.ID != ""
	case "data.srcip":
		return source.Data.SrcIP, source.Data.SrcIP != ""
	case "data.srcuser":
		return source.Data.SrcUser, source.Data.SrcUser != ""
	case "data.dstuser":
		return source.Data.DstUser, source.Data.DstUser != ""
	case "data.url":
		return source.Data.URL, source.Data.URL != ""
	case "decoder.name":
		return source.Decoder.Name, source.Decoder.Name != ""
	case "location":
		return source.Location, source.Location != ""
	default:
		return nil, false
	}
}

func (provider *Provider) ResolveContext(ctx context.Context, _ capability.Access, request capability.ResolveContextRequest) (capability.ContextPage, error) {
	if provider == nil || provider.client == nil {
		return capability.ContextPage{}, sourceRequestError("source_unavailable", "Wazuh client is not configured")
	}
	if len(request.EntityIDs) > 0 {
		return capability.ContextPage{}, sourceRequestError("invalid_source_request", "Wazuh entity resolve is not supported")
	}
	page := capability.ContextPage{}
	fetchedAt := provider.client.now().UTC()
	for _, eventID := range dedupeStrings(request.EventIDs) {
		index, documentID, err := splitEventID(eventID)
		if err != nil {
			return capability.ContextPage{}, err
		}
		doc, getErr := provider.client.GetDocument(ctx, index, documentID)
		if getErr != nil {
			return capability.ContextPage{}, translateError(getErr)
		}
		mapped, mapErr := mapHit(doc.Index, doc.ID, doc.Source, fetchedAt)
		if mapErr != nil {
			return capability.ContextPage{}, translateError(mapErr)
		}
		page.Events = append(page.Events, mapped.Event)
		page.Entities = append(page.Entities, mapped.Entities...)
		page.Relations = append(page.Relations, mapped.Relations...)
	}
	page.Events = normalization.Events(page.Events)
	page.Entities = normalization.Entities(page.Entities)
	page.Relations = normalization.Relations(page.Relations)
	return page, nil
}

func (provider *Provider) Probe(ctx context.Context, _ capability.Access) (string, error) {
	if provider == nil || provider.client == nil {
		return "offline", sourceRequestError("source_unavailable", "Wazuh client is not configured")
	}
	health, err := provider.client.ClusterHealth(ctx)
	if err != nil {
		return "offline", translateError(err)
	}
	switch strings.ToLower(strings.TrimSpace(health.Status)) {
	case "green", "yellow":
		return "online", nil
	case "red":
		return "degraded", nil
	default:
		return "offline", nil
	}
}

func (provider *Provider) AggregateEvents(ctx context.Context, _ capability.Access, request capability.AggregateEventsRequest) (capability.EventGroupPage, error) {
	if provider == nil || provider.client == nil {
		return capability.EventGroupPage{}, sourceRequestError("source_unavailable", "Wazuh client is not configured")
	}
	body, err := buildAggregateRequest(provider.client, request)
	if err != nil {
		return capability.EventGroupPage{}, translateError(err)
	}
	response, err := provider.client.Search(ctx, body)
	if err != nil {
		return capability.EventGroupPage{}, translateError(err)
	}
	groups, err := mapAggregateResponse(request.GroupBy, response)
	if err != nil {
		return capability.EventGroupPage{}, translateError(err)
	}
	status := "complete"
	if response.TimedOut || response.Shards.Failed > 0 {
		status = "truncated"
	}
	return capability.EventGroupPage{Groups: groups, Status: status}, nil
}

func buildAggregateRequest(client *Client, request capability.AggregateEventsRequest) (searchRequest, error) {
	if request.TimeFrom.IsZero() || request.TimeTo.IsZero() || !request.TimeFrom.Before(request.TimeTo) {
		return searchRequest{}, &RequestError{Operation: "event aggregate", Message: "time_from must be earlier than time_to"}
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return searchRequest{}, &RequestError{Operation: "event aggregate", Message: "limit must be between 1 and 1000"}
	}
	if len(request.GroupBy) == 0 {
		return searchRequest{}, &RequestError{Operation: "event aggregate", Message: "group_by is required"}
	}
	filters := []any{
		map[string]any{"range": map[string]any{"@timestamp": map[string]any{
			"gte": request.TimeFrom.UTC().Format(time.RFC3339Nano),
			"lte": request.TimeTo.UTC().Format(time.RFC3339Nano),
		}}},
	}
	entityQuery, _, err := entityFilterQuery(request.Entities)
	if err != nil {
		return searchRequest{}, err
	}
	if entityQuery != nil {
		filters = append(filters, entityQuery)
	}
	filterQuery, err := filterToQuery(request.Filter)
	if err != nil {
		return searchRequest{}, err
	}
	if filterQuery != nil {
		filters = append(filters, filterQuery)
	}
	aggs, err := aggregateDefinition(request.GroupBy, request.Sort, request.Limit)
	if err != nil {
		return searchRequest{}, err
	}
	return searchRequest{
		Size:           0,
		TrackTotalHits: false,
		Timeout:        searchTimeout,
		Query:          map[string]any{"bool": map[string]any{"filter": filters}},
		Sort:           []map[string]any{{"_doc": map[string]any{"order": "asc"}}},
		Source:         sourceFilter{Includes: []string{"id"}},
		Aggregations:   aggs,
	}, nil
}

func aggregateDefinition(groupBy []string, sortRules []capability.EventSort, limit int) (map[string]any, error) {
	if len(groupBy) == 1 && strings.TrimSpace(groupBy[0]) == "correlation_type" {
		return map[string]any{
			"groups": map[string]any{
				"filters": map[string]any{
					"filters": map[string]any{
						"wazuh_frequency": map[string]any{"exists": map[string]any{"field": "rule.frequency"}},
						"wazuh_rule": map[string]any{"bool": map[string]any{
							"must_not": []any{map[string]any{"exists": map[string]any{"field": "rule.frequency"}}},
						}},
					},
				},
			},
		}, nil
	}
	paths := make([]string, 0, len(groupBy))
	for _, field := range groupBy {
		spec, ok := lookupField(field)
		if !ok || spec.Path == "" || spec.CorrelationType || spec.DocumentID {
			return nil, &RequestError{Operation: "event aggregate", Message: "group_by contains an unsupported field"}
		}
		paths = append(paths, spec.Path)
	}
	orderField := "_count"
	orderDir := "desc"
	for _, rule := range sortRules {
		field := strings.TrimSpace(rule.Field)
		direction := strings.ToLower(strings.TrimSpace(rule.Direction))
		if direction != "asc" && direction != "desc" {
			return nil, &RequestError{Operation: "event aggregate", Message: "sort direction must be asc or desc"}
		}
		switch field {
		case "count":
			orderField, orderDir = "_count", direction
		default:
			if field == groupBy[0] {
				orderField, orderDir = "_key", direction
			} else {
				return nil, &RequestError{Operation: "event aggregate", Message: "sort contains an unsupported field"}
			}
		}
	}
	if len(paths) == 1 {
		return map[string]any{
			"groups": map[string]any{
				"terms": map[string]any{
					"field": paths[0],
					"size":  limit,
					"order": map[string]any{orderField: orderDir},
				},
			},
		}, nil
	}
	sources := make([]map[string]any, 0, len(paths))
	for index, pathValue := range paths {
		sources = append(sources, map[string]any{
			fmt.Sprintf("g%d", index): map[string]any{"terms": map[string]any{"field": pathValue}},
		})
	}
	return map[string]any{
		"groups": map[string]any{
			"composite": map[string]any{
				"size":    limit,
				"sources": sources,
			},
		},
	}, nil
}

func mapAggregateResponse(groupBy []string, response searchResponse) ([]domain.EventGroup, error) {
	raw, ok := response.Aggregations["groups"]
	if !ok {
		return nil, &ResponseError{Operation: "event aggregate", Message: "aggregation is missing"}
	}
	if len(groupBy) == 1 && strings.TrimSpace(groupBy[0]) == "correlation_type" {
		var filters filtersAggregation
		if err := json.Unmarshal(raw, &filters); err != nil {
			return nil, &ResponseError{Operation: "event aggregate", Message: "aggregation is invalid"}
		}
		groups := make([]domain.EventGroup, 0, len(filters.Buckets))
		for key, bucket := range filters.Buckets {
			value := key
			groups = append(groups, domain.EventGroup{SourceCode: SourceCode, Values: []*string{&value}, Count: bucket.DocCount})
		}
		sort.Slice(groups, func(i, j int) bool { return groups[i].Count > groups[j].Count })
		return groups, nil
	}
	if len(groupBy) == 1 {
		var terms termsAggregation
		if err := json.Unmarshal(raw, &terms); err != nil {
			return nil, &ResponseError{Operation: "event aggregate", Message: "aggregation is invalid"}
		}
		groups := make([]domain.EventGroup, 0, len(terms.Buckets))
		for _, bucket := range terms.Buckets {
			value := formatAggKey(bucket.Key)
			groups = append(groups, domain.EventGroup{SourceCode: SourceCode, Values: []*string{&value}, Count: bucket.DocCount})
		}
		return groups, nil
	}
	var composite compositeAggregation
	if err := json.Unmarshal(raw, &composite); err != nil {
		return nil, &ResponseError{Operation: "event aggregate", Message: "aggregation is invalid"}
	}
	groups := make([]domain.EventGroup, 0, len(composite.Buckets))
	for _, bucket := range composite.Buckets {
		values := make([]*string, 0, len(groupBy))
		for index := range groupBy {
			rawValue, exists := bucket.Key[fmt.Sprintf("g%d", index)]
			if !exists || rawValue == nil {
				values = append(values, nil)
				continue
			}
			value := formatAggKey(rawValue)
			values = append(values, &value)
		}
		groups = append(groups, domain.EventGroup{SourceCode: SourceCode, Values: values, Count: bucket.DocCount})
	}
	return groups, nil
}

func formatAggKey(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func splitEventID(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	index, documentID, ok := strings.Cut(value, "/")
	if !ok || strings.TrimSpace(index) == "" || strings.TrimSpace(documentID) == "" {
		return "", "", sourceRequestError("invalid_source_ref", "Wazuh source event ID is invalid")
	}
	if strings.Contains(documentID, "/") {
		return "", "", sourceRequestError("invalid_source_ref", "Wazuh source event ID is invalid")
	}
	return index, documentID, nil
}

func dedupeStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func sourceRequestError(code, message string) error {
	return &domain.RequestError{Code: code, Message: message}
}

func translateError(err error) error {
	if err == nil {
		return nil
	}
	var notFound *NotFoundError
	if errors.As(err, &notFound) {
		return fmt.Errorf("%w: Wazuh record %s", domain.ErrNotFound, notFound.ExternalID)
	}
	var upstream *HTTPError
	if errors.As(err, &upstream) {
		if upstream.StatusCode == http.StatusBadRequest {
			return sourceRequestError("invalid_source_request", "Wazuh rejected the request")
		}
		return &domain.UpstreamError{StatusCode: upstream.StatusCode, Message: "Wazuh source request failed"}
	}
	var response *ResponseError
	if errors.As(err, &response) {
		return &domain.UpstreamError{StatusCode: http.StatusBadGateway, Message: "Wazuh source response is invalid"}
	}
	var request *RequestError
	if errors.As(err, &request) {
		code := "invalid_source_request"
		if strings.Contains(request.Message, "unsupported_entity_type") {
			code = "unsupported_entity_type"
		}
		return sourceRequestError(code, request.Message)
	}
	var transport *TransportError
	if errors.As(err, &transport) {
		if transport.TimedOut {
			return context.DeadlineExceeded
		}
		return &domain.UpstreamError{StatusCode: http.StatusBadGateway, Message: "Wazuh source request failed"}
	}
	return err
}
