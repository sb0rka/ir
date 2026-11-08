package maxpatrol

import (
	"sort"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
	"github.com/sb0rka/ir/apps/gateway/internal/fieldmap"
)

const EventFilterLanguage = "pt_pdql"

func (provider *Provider) EventQueryDescription() capability.EventQueryDescription {
	result := capability.EventQueryDescription{Languages: []string{EventFilterLanguage}, CommonFilter: true}
	for name := range eventSearchAllowedFields() {
		kind := "string"
		ops := []string{"=", "!=", ">", ">=", "<", "<=", "contains", "is_null", "is_not_null"}
		switch name {
		case "time", "recv_time":
			kind = "datetime"
			ops = []string{"=", "!=", ">", ">=", "<", "<=", "is_null", "is_not_null"}
		case "importance", "src.port", "dst.port":
			kind = "number"
			ops = []string{"=", "!=", ">", ">=", "<", "<=", "is_null", "is_not_null"}
		case "src.ip", "dst.ip", "event_src.ip":
			kind = "ip"
			ops = []string{"=", "!=", "is_null", "is_not_null"}
		}
		result.Fields = append(result.Fields, capability.EventField{Name: name, Type: kind, Operators: ops, Column: true, Sort: true, Group: true})
	}
	sort.Slice(result.Fields, func(i, j int) bool { return result.Fields[i].Name < result.Fields[j].Name })
	return result
}
func (provider *Provider) ValidateEventQuery(request capability.SearchEventsRequest, aggregate bool) error {
	if request.FilterLanguage != "" && request.FilterLanguage != EventFilterLanguage {
		return sourceRequestError("unsupported_filter_language", "MaxPatrol requires pt_pdql")
	}
	if request.FilterLanguage == EventFilterLanguage {
		if err := validateNativePredicate(request.Filter); err != nil {
			return err
		}
	}
	where, err := eventQueryWhere(request.Entities, request.CommonFilter)
	if err != nil {
		return err
	}
	if aggregate {
		_, err = buildEventAggregationQuery(capability.AggregateEventsRequest{TimeFrom: request.TimeFrom, TimeTo: request.TimeTo, Filter: request.Filter, GroupBy: request.GroupBy, Sort: request.Sort, Limit: request.Limit}, where)
	} else {
		_, err = buildEventSearchQuery(request, where)
	}
	return eventQueryError(err)
}
func eventQueryError(err error) error {
	if typed, ok := err.(*RequestError); ok {
		code := "unsupported_query_control"
		if strings.Contains(typed.Message, "filter field") {
			code = "unsupported_filter_field"
		}
		return sourceRequestError(code, typed.Message)
	}
	return err
}
func eventQueryWhere(entities []domain.EntityRef, filter fieldmap.CommonFilter) (string, error) {
	where, err := eventWhere(entities)
	if err != nil {
		return "", err
	}
	for _, item := range []struct {
		kind   string
		values []string
	}{{"host", filter.Hosts}, {"ip", filter.IPs}} {
		if len(item.values) == 0 {
			continue
		}
		refs := []domain.EntityRef{}
		for _, v := range item.values {
			refs = append(refs, domain.EntityRef{Type: item.kind, Value: v})
		}
		part, err := eventWhere(refs)
		if err != nil {
			return "", err
		}
		where = "(" + where + ") and (" + part + ")"
	}
	return where, nil
}
