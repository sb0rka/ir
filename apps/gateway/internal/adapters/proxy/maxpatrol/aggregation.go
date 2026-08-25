package maxpatrol

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

const eventAggregationPath = "api/events/v3/events/aggregation"

type eventAggregationQuery struct {
	PDQL  string
	Limit int
}

type eventAggregationRequest struct {
	Filter   string `json:"filter"`
	TimeFrom int64  `json:"timeFrom"`
	TimeTo   int64  `json:"timeTo"`
}

type eventAggregationResponse struct {
	Errors         json.RawMessage       `json:"errors"`
	HasMoreResults bool                  `json:"hasMoreResults"`
	Columns        []string              `json:"columns"`
	Rows           []eventAggregationRow `json:"rows"`
}

type eventAggregationRow struct {
	Groups []*string     `json:"groups"`
	Values []json.Number `json:"values"`
}

func buildEventAggregationQuery(request capability.AggregateEventsRequest, entityWhere string) (eventAggregationQuery, error) {
	allowed := eventSearchAllowedFields()
	where, err := eventSearchWhere(entityWhere, request.Filter, allowed)
	if err != nil {
		return eventAggregationQuery{}, err
	}
	groupBy, err := validateEventFields("group_by", request.GroupBy, allowed)
	if err != nil {
		return eventAggregationQuery{}, err
	}
	if len(groupBy) < 1 || len(groupBy) > 8 {
		return eventAggregationQuery{}, requestError("event aggregation", "group_by must contain between 1 and 8 fields")
	}

	sortRules := append([]capability.EventSort(nil), request.Sort...)
	if len(sortRules) == 0 {
		sortRules = []capability.EventSort{{Field: "count", Direction: "desc"}}
	}
	seenSort := make(map[string]struct{}, len(sortRules))
	for index := range sortRules {
		field := strings.TrimSpace(sortRules[index].Field)
		direction := strings.ToLower(strings.TrimSpace(sortRules[index].Direction))
		if field != "count" && !containsEventField(groupBy, field) {
			return eventAggregationQuery{}, requestError("event aggregation", "sort field must be count or a group_by field")
		}
		if direction != "asc" && direction != "desc" {
			return eventAggregationQuery{}, requestError("event aggregation", "sort direction must be asc or desc")
		}
		if _, duplicate := seenSort[field]; duplicate {
			return eventAggregationQuery{}, requestError("event aggregation", "sort fields must be unique")
		}
		seenSort[field] = struct{}{}
		if field == "count" {
			field = "Cnt"
		}
		sortRules[index] = capability.EventSort{Field: field, Direction: direction}
	}
	for _, field := range groupBy {
		if _, exists := seenSort[field]; exists {
			continue
		}
		sortRules = append(sortRules, capability.EventSort{Field: field, Direction: "asc"})
	}

	parts := []string{
		"filter(" + where + ")",
		"select(" + strings.Join(groupBy, ", ") + ")",
		"group(key: [" + strings.Join(groupBy, ", ") + "], agg: COUNT(*) as Cnt)",
		"sort(" + eventSortPDQL(sortRules) + ")",
		fmt.Sprintf("limit(%d)", request.Limit),
	}
	return eventAggregationQuery{PDQL: strings.Join(parts, " | "), Limit: request.Limit}, nil
}

func (client *Client) aggregateEventsWithQuery(ctx context.Context, access Access, timeRange TimeRange, query eventAggregationQuery) (eventAggregationResponse, error) {
	if err := timeRange.validate(); err != nil {
		return eventAggregationResponse{}, err
	}
	request := eventAggregationRequest{Filter: query.PDQL, TimeFrom: timeRange.From.Unix(), TimeTo: timeRange.To.Unix()}
	var response eventAggregationResponse
	if err := client.doJSON(ctx, client.siem, access, "event aggregation", http.MethodPost, eventAggregationPath, nil, request, &response); err != nil {
		return eventAggregationResponse{}, err
	}
	if nonNullVendorErrors(response.Errors) {
		return eventAggregationResponse{}, &ResponseError{Operation: "event aggregation", Message: "vendor reported query errors"}
	}
	if len(response.Columns) != 1 || response.Columns[0] != "COUNT" {
		return eventAggregationResponse{}, &ResponseError{Operation: "event aggregation", Message: "aggregation columns are invalid"}
	}
	return response, nil
}

func (provider *Provider) AggregateEvents(ctx context.Context, access capability.Access, request capability.AggregateEventsRequest) (capability.EventGroupPage, error) {
	if request.TimeFrom.IsZero() || request.TimeTo.IsZero() || !request.TimeFrom.Before(request.TimeTo) {
		return capability.EventGroupPage{}, sourceRequestError("invalid_time_range", "time_from must be earlier than time_to")
	}
	if request.Limit < 1 || request.Limit > 1000 {
		return capability.EventGroupPage{}, sourceRequestError("invalid_limit", "limit must be between 1 and 1000")
	}
	where, err := eventWhere(request.Entities)
	if err != nil {
		return capability.EventGroupPage{}, err
	}
	query, err := buildEventAggregationQuery(request, where)
	if err != nil {
		return capability.EventGroupPage{}, err
	}
	response, err := provider.client.aggregateEventsWithQuery(ctx, Access{Cookie: access.Cookie}, TimeRange{From: request.TimeFrom, To: request.TimeTo}, query)
	if err != nil {
		return capability.EventGroupPage{}, translateError(err)
	}

	page := capability.EventGroupPage{Status: "complete"}
	if response.HasMoreResults || len(response.Rows) >= request.Limit {
		page.Status = "truncated"
	}
	rows := response.Rows
	if len(rows) > request.Limit {
		rows = rows[:request.Limit]
		page.Status = "truncated"
	}
	for _, row := range rows {
		if len(row.Groups) != len(request.GroupBy) || len(row.Values) != 1 {
			return capability.EventGroupPage{}, translateError(&ResponseError{Operation: "event aggregation", Message: "aggregation row shape is invalid"})
		}
		for _, value := range row.Groups {
			if value != nil && (len(*value) > 1024 || containsControl(*value)) {
				return capability.EventGroupPage{}, translateError(&ResponseError{Operation: "event aggregation", Message: "aggregation group value is invalid"})
			}
		}
		count, countErr := aggregationCount(row.Values[0])
		if countErr != nil {
			return capability.EventGroupPage{}, translateError(&ResponseError{Operation: "event aggregation", Message: "aggregation count is invalid"})
		}
		page.Groups = append(page.Groups, domain.EventGroup{
			SourceCode: SourceCode,
			Values:     append([]*string(nil), row.Groups...),
			Count:      count,
		})
	}
	return page, nil
}

func aggregationCount(value json.Number) (int64, error) {
	raw := strings.TrimSpace(value.String())
	parsed, ok := new(big.Rat).SetString(raw)
	if !ok || !parsed.IsInt() || parsed.Sign() < 0 || !parsed.Num().IsInt64() {
		return 0, fmt.Errorf("count is not a non-negative int64")
	}
	return parsed.Num().Int64(), nil
}
