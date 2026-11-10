package wazuh

import (
	"bytes"
	"container/heap"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

type groupHeap struct {
	items   []domain.EventGroup
	compare func(domain.EventGroup, domain.EventGroup) int
}

func (h groupHeap) Len() int           { return len(h.items) }
func (h groupHeap) Less(i, j int) bool { return h.compare(h.items[i], h.items[j]) > 0 }
func (h groupHeap) Swap(i, j int)      { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *groupHeap) Push(v any)        { h.items = append(h.items, v.(domain.EventGroup)) }
func (h *groupHeap) Pop() any {
	v := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	return v
}

func (provider *Provider) AggregateEvents(ctx context.Context, access capability.Access, request capability.AggregateEventsRequest) (capability.EventGroupPage, error) {
	page := capability.EventGroupPage{Status: "complete"}
	if request.Limit < 1 || request.Limit > 1000 || len(request.GroupBy) < 1 || len(request.GroupBy) > 8 {
		return page, sourceRequestError("invalid_source_request", "aggregation limits are invalid")
	}
	query, err := provider.eventQuery(capability.SearchEventsRequest{TimeFrom: request.TimeFrom, TimeTo: request.TimeTo, Entities: request.Entities, CommonFilter: request.CommonFilter, Filter: request.Filter, FilterLanguage: request.FilterLanguage})
	if err != nil {
		return page, translateError(err)
	}
	sources := []map[string]any{}
	groupIndex := map[string]int{}
	for i, field := range request.GroupBy {
		spec, ok := lookupField(field)
		if !ok {
			return page, sourceRequestError("invalid_source_request", "group_by contains an unsupported field")
		}
		if _, exists := groupIndex[field]; exists {
			return page, sourceRequestError("invalid_source_request", "group_by fields must be unique")
		}
		groupIndex[field] = i
		sources = append(sources, map[string]any{fmt.Sprintf("g%d", i): map[string]any{"terms": map[string]any{"field": spec.Path, "missing_bucket": true}}})
	}
	rules := append([]capability.EventSort(nil), request.Sort...)
	if len(rules) == 0 {
		rules = []capability.EventSort{{Field: "count", Direction: "desc"}}
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		_, ok := groupIndex[rule.Field]
		if (rule.Field != "count" && !ok) || (rule.Direction != "asc" && rule.Direction != "desc") || seen[rule.Field] {
			return page, sourceRequestError("invalid_source_request", "aggregation sort is invalid")
		}
		seen[rule.Field] = true
	}
	for _, field := range request.GroupBy {
		if !seen[field] {
			rules = append(rules, capability.EventSort{Field: field, Direction: "asc"})
		}
	}
	compare := func(a, b domain.EventGroup) int {
		for _, rule := range rules {
			cmp := 0
			if rule.Field == "count" {
				if a.Count < b.Count {
					cmp = -1
				} else if a.Count > b.Count {
					cmp = 1
				}
			} else {
				i := groupIndex[rule.Field]
				x, y := a.Values[i], b.Values[i]
				if x == nil && y != nil {
					return 1
				}
				if x != nil && y == nil {
					return -1
				}
				if x == nil {
					continue
				}
				spec, _ := lookupField(rule.Field)
				if spec.Type == fieldLong {
					left, lok := new(big.Rat).SetString(*x)
					right, rok := new(big.Rat).SetString(*y)
					if lok && rok {
						cmp = left.Cmp(right)
					} else {
						cmp = strings.Compare(*x, *y)
					}
				} else {
					cmp = strings.Compare(*x, *y)
				}
			}
			if cmp != 0 {
				if rule.Direction == "desc" {
					return -cmp
				}
				return cmp
			}
		}
		return 0
	}
	pit, err := provider.client.openPIT(ctx, access)
	if err != nil {
		return page, err
	}
	defer provider.release(ctx, access, pit)
	top := &groupHeap{compare: compare}
	heap.Init(top)
	var after map[string]any
	var previous string
	count := 0
	for {
		composite := map[string]any{"size": 100, "sources": sources}
		if after != nil {
			composite["after"] = after
		}
		response, err := provider.client.search(ctx, access, searchRequest{Size: 0, PIT: &pointInTime{ID: pit, KeepAlive: pitKeepAlive}, Query: query, Source: sourceFilter{Includes: []string{"id"}}, Aggregations: map[string]any{"groups": map[string]any{"composite": composite}}})
		if err != nil {
			return capability.EventGroupPage{}, err
		}
		if response.TimedOut || response.Shards.Failed > 0 {
			return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation is incomplete"}
		}
		var groups compositeAggregation
		raw, ok := response.Aggregations["groups"]
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		if !ok || d.Decode(&groups) != nil {
			return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation response is invalid"}
		}
		for _, bucket := range groups.Buckets {
			if bucket.DocCount < 0 || len(bucket.Key) != len(request.GroupBy) {
				return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation bucket is invalid"}
			}
			group := domain.EventGroup{SourceCode: SourceCode, Count: bucket.DocCount}
			for i := range request.GroupBy {
				value, exists := bucket.Key[fmt.Sprintf("g%d", i)]
				if !exists {
					return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation key is invalid"}
				}
				if value == nil {
					group.Values = append(group.Values, nil)
					continue
				}
				var text string
				switch v := value.(type) {
				case string:
					text = v
				case json.Number:
					text = v.String()
				case bool:
					text = fmt.Sprint(v)
				default:
					return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation key is invalid"}
				}
				if len(text) > 1024 || strings.ContainsAny(text, "\r\n\x00") {
					return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation key is invalid"}
				}
				group.Values = append(group.Values, &text)
			}
			count++
			if top.Len() < request.Limit {
				heap.Push(top, group)
			} else if compare(group, top.items[0]) < 0 {
				heap.Pop(top)
				heap.Push(top, group)
			}
		}
		if len(groups.Buckets) == 0 || len(groups.AfterKey) == 0 {
			break
		}
		encoded, _ := json.Marshal(groups.AfterKey)
		if string(encoded) == previous {
			return capability.EventGroupPage{}, &domain.UpstreamError{StatusCode: 502, Message: "Wazuh aggregation did not advance"}
		}
		previous = string(encoded)
		after = groups.AfterKey
	}
	page.Groups = make([]domain.EventGroup, top.Len())
	for i := len(page.Groups) - 1; i >= 0; i-- {
		page.Groups[i] = heap.Pop(top).(domain.EventGroup)
	}
	if count > request.Limit {
		page.Status = "truncated"
	}
	return page, nil
}
