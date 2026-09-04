package maxpatrol

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"

	"github.com/sb0rka/ir/apps/gateway/internal/capability"
)

const maxEventFilterLength = 4096

var eventSearchExtraFields = []string{
	"subject",
	"subject.account.provider",
	"subject.account.session_id",
	"subject.process.id",
	"subject.process.chain",
	"object",
	"object.process.id",
	"object.process.chain",
	"incident.id",
}

type eventSearchQuery struct {
	PDQL        string
	GroupValues []*string
	Limit       int
	// Offset continues the same PDQL result set; the SIEM orders pages by the
	// query sort, so offset paging is stable for a fixed time window.
	Offset int
}

type eventSearchQueryV3Request struct {
	Filter      string    `json:"filter"`
	GroupValues []*string `json:"groupValues"`
	TimeFrom    int64     `json:"timeFrom"`
	TimeTo      int64     `json:"timeTo"`
}

func buildEventSearchQuery(request capability.SearchEventsRequest, entityWhere string) (eventSearchQuery, error) {
	allowed := eventSearchAllowedFields()
	where, err := eventSearchWhere(entityWhere, request.Filter, allowed)
	if err != nil {
		return eventSearchQuery{}, err
	}
	columns := append([]string(nil), request.Columns...)
	if len(columns) == 0 {
		columns = append(columns, correlationSelect...)
	}
	columns, err = validateEventFields("columns", columns, allowed)
	if err != nil {
		return eventSearchQuery{}, err
	}
	for _, required := range []string{"time", "uuid", "text", "importance"} {
		if !containsEventField(columns, required) {
			columns = append(columns, required)
		}
	}

	sortRules := append([]capability.EventSort(nil), request.Sort...)
	if len(sortRules) == 0 {
		sortRules = []capability.EventSort{{Field: "time", Direction: "desc"}}
	}
	for index := range sortRules {
		sortRules[index].Field = strings.TrimSpace(sortRules[index].Field)
		sortRules[index].Direction = strings.ToLower(strings.TrimSpace(sortRules[index].Direction))
		if _, ok := allowed[sortRules[index].Field]; !ok {
			return eventSearchQuery{}, requestError("event search", "sort contains an unsupported field")
		}
		if sortRules[index].Direction != "asc" && sortRules[index].Direction != "desc" {
			return eventSearchQuery{}, requestError("event search", "sort direction must be asc or desc")
		}
		if !containsEventField(columns, sortRules[index].Field) {
			columns = append(columns, sortRules[index].Field)
		}
	}

	groupBy, err := validateEventFields("group_by", request.GroupBy, allowed)
	if err != nil {
		return eventSearchQuery{}, err
	}
	if len(request.GroupValues) != len(groupBy) {
		return eventSearchQuery{}, requestError("event search", "group_values must align with group_by")
	}
	for _, value := range request.GroupValues {
		if value != nil && (len(*value) > 1024 || containsControl(*value)) {
			return eventSearchQuery{}, requestError("event search", "group_values contains an invalid value")
		}
	}

	parts := []string{
		"filter(" + where + ")",
		"select(" + strings.Join(columns, ", ") + ")",
		"sort(" + eventSortPDQL(sortRules) + ")",
	}
	if len(groupBy) > 0 {
		parts = append(parts,
			"group(key: ["+strings.Join(groupBy, ", ")+"], agg: COUNT(*) as Cnt)",
			"sort(Cnt desc)",
		)
	}
	// Do not put limit() in PDQL: MaxPatrol totalCount is computed after the
	// pipeline, so a PDQL limit would collapse the reported total to the page size.
	// Page size is applied via the HTTP limit/offset query parameters instead.
	groupValues := append([]*string(nil), request.GroupValues...)
	if len(groupBy) == 0 {
		defaultGroup := "1"
		groupValues = []*string{&defaultGroup}
	}
	return eventSearchQuery{PDQL: strings.Join(parts, " | "), GroupValues: groupValues, Limit: request.Limit}, nil
}

func (client *Client) searchEventsWithQuery(ctx context.Context, access Access, timeRange TimeRange, querySpec eventSearchQuery) (safeEventsEnvelope, error) {
	if err := timeRange.validate(); err != nil {
		return safeEventsEnvelope{}, err
	}
	request := eventSearchQueryV3Request{
		Filter:      querySpec.PDQL,
		GroupValues: querySpec.GroupValues,
		TimeFrom:    timeRange.From.Unix(),
		TimeTo:      timeRange.To.Unix(),
	}
	query := url.Values{}
	query.Set("offset", fmt.Sprintf("%d", max(0, querySpec.Offset)))
	query.Set("limit", fmt.Sprintf("%d", querySpec.Limit))
	query.Set("recursive", "true")
	// The first page requests a real totalCount so the public search response
	// can expose match totals; continuation pages skip the count to keep the
	// SIEM from recounting the same result set.
	query.Set("noCount", strconv.FormatBool(querySpec.Offset > 0))
	var response safeEventsEnvelope
	if err := client.doJSON(ctx, client.siem, access, "event search", http.MethodPost, eventsPath, query, request, &response); err != nil {
		return safeEventsEnvelope{}, err
	}
	if nonNullVendorErrors(response.Errors) {
		return safeEventsEnvelope{}, &ResponseError{Operation: "event search", Message: "vendor reported query errors"}
	}
	if err := finalizeEventsEnvelope(&response); err != nil {
		return safeEventsEnvelope{}, &ResponseError{Operation: "event search", Message: err.Error()}
	}
	return response, nil
}

func eventSearchWhere(entityWhere, filter string, allowed map[string]struct{}) (string, error) {
	entityWhere = strings.TrimSpace(entityWhere)
	filter = strings.TrimSpace(filter)
	if err := validateEventPredicate(filter, allowed); err != nil {
		return "", err
	}
	if filter == "" {
		return entityWhere, nil
	}
	if entityWhere == "" || entityWhere == "uuid != null" {
		return filter, nil
	}
	return "(" + entityWhere + ") and (" + filter + ")", nil
}

func validateEventPredicate(value string, allowed map[string]struct{}) error {
	if len(value) > maxEventFilterLength {
		return requestError("event search", "filter is too long")
	}
	if value == "" {
		return nil
	}
	if strings.ContainsAny(value, "|;\r\n\x00") || strings.Contains(value, "--") || strings.Contains(value, "/*") || strings.Contains(value, "*/") {
		return requestError("event search", "filter must be a predicate without query pipelines or comments")
	}
	depth := 0
	quoted := false
	escaped := false
	for _, character := range value {
		if escaped {
			escaped = false
			continue
		}
		if quoted && character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth < 0 {
				return requestError("event search", "filter parentheses are unbalanced")
			}
		}
	}
	if quoted || escaped || depth != 0 {
		return requestError("event search", "filter quotes or parentheses are unbalanced")
	}
	if err := validateEventPredicateFields(value, allowed); err != nil {
		return err
	}
	return nil
}

func validateEventPredicateFields(value string, allowed map[string]struct{}) error {
	keywords := map[string]struct{}{"and": {}, "or": {}, "not": {}, "contains": {}, "null": {}, "true": {}, "false": {}}
	quoted := false
	escaped := false
	for index := 0; index < len(value); {
		character := value[index]
		if escaped {
			escaped = false
			index++
			continue
		}
		if quoted && character == '\\' {
			escaped = true
			index++
			continue
		}
		if character == '"' {
			quoted = !quoted
			index++
			continue
		}
		if quoted || !isEventIdentifierStart(character) {
			index++
			continue
		}
		end := index + 1
		for end < len(value) && isEventIdentifierPart(value[end]) {
			end++
		}
		token := value[index:end]
		if _, keyword := keywords[strings.ToLower(token)]; !keyword {
			if _, ok := allowed[token]; !ok {
				return requestError("event search", "filter contains an unsupported field")
			}
		}
		index = end
	}
	return nil
}

func isEventIdentifierStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func isEventIdentifierPart(value byte) bool {
	return isEventIdentifierStart(value) || value == '.' || value >= '0' && value <= '9'
}

func eventSearchAllowedFields() map[string]struct{} {
	result := make(map[string]struct{}, len(correlationSelect)+len(eventSearchExtraFields))
	for _, field := range correlationSelect {
		result[field] = struct{}{}
	}
	for _, field := range eventSearchExtraFields {
		result[field] = struct{}{}
	}
	return result
}

func validateEventFields(name string, values []string, allowed map[string]struct{}) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if _, ok := allowed[value]; !ok {
			return nil, requestError("event search", name+" contains an unsupported field")
		}
		if _, duplicate := seen[value]; duplicate {
			return nil, requestError("event search", name+" fields must be unique")
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func eventSortPDQL(values []capability.EventSort) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, value.Field+" "+value.Direction)
	}
	return strings.Join(parts, ", ")
}

func containsEventField(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func requestError(operation, message string) error {
	return &RequestError{Operation: operation, Message: message}
}
