package wazuh

import (
	"sort"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const EventFilterLanguage = "wazuh_dql"

func (provider *Provider) EventQueryDescription() capability.EventQueryDescription {
	result := capability.EventQueryDescription{Languages: []string{EventFilterLanguage}, CommonFilter: true}
	for name, spec := range fieldCatalog {
		kind := "string"
		ops := []string{"=", "!=", "contains", "in", "is_null", "is_not_null"}
		if spec.Type == fieldDate || spec.Type == fieldLong {
			kind = "number"
			if spec.Type == fieldDate {
				kind = "datetime"
			}
			ops = []string{"=", "!=", ">", ">=", "<", "<=", "in", "is_null", "is_not_null"}
		}
		result.Fields = append(result.Fields, capability.EventField{Name: name, Type: kind, Operators: ops, Column: true, Sort: true, Group: true})
	}
	sort.Slice(result.Fields, func(i, j int) bool { return result.Fields[i].Name < result.Fields[j].Name })
	return result
}
func (provider *Provider) eventQuery(request capability.SearchEventsRequest) (map[string]any, error) {
	if request.FilterLanguage != "" && request.FilterLanguage != EventFilterLanguage {
		return nil, sourceRequestError("unsupported_filter_language", "Wazuh requires wazuh_dql")
	}
	raw := request.Filter
	if request.FilterLanguage == EventFilterLanguage {
		raw = ""
	}
	query, err := buildQuery(request.TimeFrom, request.TimeTo, request.Entities, raw, request.GroupBy, request.GroupValues)
	if err != nil {
		return nil, translateError(err)
	}
	filters := query["bool"].(map[string]any)["filter"].([]any)
	if request.FilterLanguage == EventFilterLanguage {
		q, err := dqlToQuery(request.Filter)
		if err != nil {
			return nil, err
		}
		if q != nil {
			filters = append(filters, q)
		}
	}
	for _, item := range []struct {
		kind   string
		values []string
	}{{"host", request.CommonFilter.Hosts}, {"ip", request.CommonFilter.IPs}} {
		if len(item.values) == 0 {
			continue
		}
		refs := []domain.EntityRef{}
		for _, v := range item.values {
			refs = append(refs, domain.EntityRef{Type: item.kind, Value: v})
		}
		q, _, err := entityFilterQuery(refs)
		if err != nil {
			return nil, translateError(err)
		}
		filters = append(filters, q)
	}
	query["bool"].(map[string]any)["filter"] = filters
	return query, nil
}
func (provider *Provider) ValidateEventQuery(request capability.SearchEventsRequest, aggregate bool) error {
	queryRequest := request
	if aggregate {
		queryRequest.GroupBy = nil
		queryRequest.GroupValues = nil
	}
	if _, err := provider.eventQuery(queryRequest); err != nil {
		return err
	}
	for _, field := range append(append([]string(nil), request.Columns...), request.GroupBy...) {
		if _, ok := lookupField(field); !ok {
			return sourceRequestError("unsupported_query_control", "Wazuh does not support field "+field)
		}
	}
	if aggregate {
		seen := map[string]bool{}
		for _, field := range request.GroupBy {
			if seen[field] {
				return sourceRequestError("unsupported_query_control", "group fields must be unique")
			}
			seen[field] = true
		}
		for _, rule := range request.Sort {
			if rule.Field != "count" && !seen[rule.Field] || rule.Direction != "asc" && rule.Direction != "desc" {
				return sourceRequestError("unsupported_query_control", "invalid Wazuh aggregation sort")
			}
		}
		return nil
	}
	_, err := provider.client.buildSort(request.Sort)
	return err
}
